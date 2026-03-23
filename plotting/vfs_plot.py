import argparse
import os
import re
from collections import defaultdict

import matplotlib.pyplot as plt


EVENT_RE = re.compile(
	r"^(?P<ts>\d+)\s+\{[^}]*}\{(?P<op>vfs_read|vfs_write)}\{(?P<fields>.*)}$"
)
FIELD_RE = re.compile(r"(fname|count|ret)=([^,}]+)")


def parse_event(line):
	match = EVENT_RE.match(line.strip())
	if not match:
		return None

	fields_blob = match.group("fields")
	fields = {key: value.strip() for key, value in FIELD_RE.findall(fields_blob)}

	if "fname" not in fields or "count" not in fields or "ret" not in fields:
		return None

	try:
		timestamp = int(match.group("ts"))
		count = int(fields["count"])
		ret = int(fields["ret"])
	except ValueError:
		return None

	op = "read" if match.group("op") == "vfs_read" else "write"
	return {
		"timestamp": timestamp,
		"op": op,
		"fname": fields["fname"],
		"count": count,
		"ret": ret,
	}


def get_extension(fname):
	_, ext = os.path.splitext(fname)
	return ext.lower() if ext else "[no_ext]"


def load_events(log_path):
	events = []
	with open(log_path, "r", encoding="utf-8", errors="replace") as handle:
		for line in handle:
			event = parse_event(line)
			if event is not None:
				events.append(event)
	return events


def filter_events_by_percent_range(events, start_pct, end_pct):
	if not (0.0 <= start_pct < end_pct <= 100.0):
		raise ValueError("Range must satisfy 0 <= range-start-pct < range-end-pct <= 100")

	min_ts = min(event["timestamp"] for event in events)
	max_ts = max(event["timestamp"] for event in events)
	span = max_ts - min_ts

	if span == 0:
		return list(events)

	start_ts = min_ts + int(span * (start_pct / 100.0))
	end_ts = min_ts + int(span * (end_pct / 100.0))

	if end_pct == 100.0:
		return [event for event in events if start_ts <= event["timestamp"] <= end_ts]

	return [event for event in events if start_ts <= event["timestamp"] < end_ts]


def bucketize(events, bucket_seconds, base_ts=None):
	first_ts = base_ts if base_ts is not None else min(event["timestamp"] for event in events)
	bucket_ns = int(bucket_seconds * 1e9)
	if bucket_ns <= 0:
		raise ValueError("bucket_seconds must be > 0")

	per_bucket = defaultdict(
		lambda: {
			"read_count": 0,
			"write_count": 0,
			"read_bytes": 0,
			"write_bytes": 0,
		}
	)

	for event in events:
		ret = event["ret"]
		if ret <= 0:
			continue

		bytes_value = min(event["count"], ret)
		bucket_index = (event["timestamp"] - first_ts) // bucket_ns
		bucket = per_bucket[bucket_index]

		if event["op"] == "read":
			bucket["read_count"] += 1
			bucket["read_bytes"] += bytes_value
		else:
			bucket["write_count"] += 1
			bucket["write_bytes"] += bytes_value

	if not per_bucket:
		return [], [], [], [], []

	max_bucket = max(per_bucket)
	time_axis = [index * bucket_seconds for index in range(max_bucket + 1)]
	read_counts = [per_bucket[index]["read_count"] for index in range(max_bucket + 1)]
	write_counts = [per_bucket[index]["write_count"] for index in range(max_bucket + 1)]
	read_bytes = [per_bucket[index]["read_bytes"] for index in range(max_bucket + 1)]
	write_bytes = [per_bucket[index]["write_bytes"] for index in range(max_bucket + 1)]

	return time_axis, read_counts, write_counts, read_bytes, write_bytes


def extension_insights(events):
	by_ext = defaultdict(
		lambda: {
			"read_unique_files": set(),
			"write_unique_files": set(),
			"read_events": 0,
			"write_events": 0,
		}
	)

	for event in events:
		if event["ret"] <= 0:
			continue

		ext = get_extension(event["fname"])
		entry = by_ext[ext]
		if event["op"] == "read":
			entry["read_events"] += 1
			entry["read_unique_files"].add(event["fname"])
		else:
			entry["write_events"] += 1
			entry["write_unique_files"].add(event["fname"])

	rows = []
	for ext, entry in by_ext.items():
		rows.append(
			(
				ext,
				len(entry["read_unique_files"]),
				len(entry["write_unique_files"]),
				entry["read_events"],
				entry["write_events"],
			)
		)

	rows.sort(key=lambda item: (item[1] + item[2], item[3] + item[4]), reverse=True)
	return rows


def print_extension_table(rows, top_n):
	if not rows:
		print("No positive-ret read/write events found for extension insights.")
		return

	display_rows = rows[:top_n] if top_n > 0 else rows

	print("\nExtension insights (positive ret only):")
	print(
		f"{'Extension':<15} {'Read Files':>10} {'Write Files':>11} {'Read Events':>11} {'Write Events':>12}"
	)
	print("-" * 64)
	for ext, read_files, write_files, read_events, write_events in display_rows:
		print(
			f"{ext:<15} {read_files:>10} {write_files:>11} {read_events:>11} {write_events:>12}"
		)


def plot_metrics(time_axis, read_counts, write_counts, read_bytes, write_bytes, output_path, show):
	plt.style.use("seaborn-v0_8-whitegrid")
	fig, axes = plt.subplots(2, 1, figsize=(12, 8), sharex=True)

	axes[0].plot(time_axis, read_counts, label="Read Count", linewidth=1.6)
	axes[0].plot(time_axis, write_counts, label="Write Count", linewidth=1.6)
	axes[0].set_title("VFS Read/Write Operation Counts Over Relative Time")
	axes[0].set_ylabel("Operation Count")
	axes[0].legend()

	axes[1].plot(time_axis, read_bytes, label="Read Bytes", linewidth=1.6)
	axes[1].plot(time_axis, write_bytes, label="Write Bytes", linewidth=1.6)
	axes[1].set_title("VFS Read/Write Bytes Over Relative Time")
	axes[1].set_xlabel("Relative Time (seconds)")
	axes[1].set_ylabel("Bytes")
	axes[1].legend()

	fig.tight_layout()
	fig.savefig(output_path, dpi=150)
	print(f"Saved plot: {output_path}")

	if show:
		plt.show()
	else:
		plt.close(fig)


def main():
	parser = argparse.ArgumentParser(
		description=(
			"Parse VFS trace logs and plot read/write counts and bytes over relative time. "
			"Only positive-ret operations are included in metrics."
		)
	)
	parser.add_argument(
		"--log",
		default="logs/vfs/trace_0.log",
		help="Path to VFS trace log file",
	)
	parser.add_argument(
		"--bucket-seconds",
		type=float,
		default=1.0,
		help="Time bucket size in seconds for aggregation (default: 1.0)",
	)
	parser.add_argument(
		"--top-ext",
		type=int,
		default=20,
		help="How many extension rows to print (0 for all)",
	)
	parser.add_argument(
		"--output",
		default="logs/vfs/vfs_rw_over_time.png",
		help="Output PNG path for plots",
	)
	parser.add_argument(
		"--show",
		action="store_true",
		help="Display plot window in addition to saving",
	)
	parser.add_argument(
		"--range-start-pct",
		type=float,
		default=0.0,
		help="Start of timeline range as percentage (default: 0)",
	)
	parser.add_argument(
		"--range-end-pct",
		type=float,
		default=100.0,
		help="End of timeline range as percentage (default: 100)",
	)
	args = parser.parse_args()

	events = load_events(args.log)
	if not events:
		raise SystemExit("No parseable VFS read/write events were found in the provided log.")

	base_ts = min(event["timestamp"] for event in events)
	events = filter_events_by_percent_range(
		events, args.range_start_pct, args.range_end_pct
	)
	if not events:
		raise SystemExit("No events found in the selected percentage range.")

	print(
		f"Using timeline range: {args.range_start_pct:.2f}% to {args.range_end_pct:.2f}%"
	)

	time_axis, read_counts, write_counts, read_bytes, write_bytes = bucketize(
		events, args.bucket_seconds, base_ts=base_ts
	)

	if not time_axis:
		raise SystemExit("No positive-ret read/write events found to plot.")

	os.makedirs(os.path.dirname(args.output) or ".", exist_ok=True)
	plot_metrics(
		time_axis,
		read_counts,
		write_counts,
		read_bytes,
		write_bytes,
		args.output,
		args.show,
	)

	rows = extension_insights(events)
	print_extension_table(rows, args.top_ext)


if __name__ == "__main__":
	main()
