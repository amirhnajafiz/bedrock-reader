import argparse
import math
import re
from collections import defaultdict
from itertools import cycle
from pathlib import Path

import matplotlib.pyplot as plt


RUNNER_LINE_RE = re.compile(r"^\[runner-\d+]\s+\[\d+]\s+")
METRIC_LINE_RE = re.compile(r"^vllm:(?P<name>[A-Za-z0-9_]+)\s*=\s*(?P<value>[-+]?\d*\.?\d+)$")
SUMMARY_START_RE = re.compile(r"^---\s+.*:\s+\d+\s+requests,\s+\d+\s+ok,\s+\d+\s+failed\s+---$")
SUMMARY_END_RE = re.compile(r"^---\s+end time:")
SUMMARY_NUMBER_RE = re.compile(r"^(?P<name>[A-Za-z0-9 %_.-]+):\s*(?P<value>[-+]?\d*\.?\d+)")

PROMPT_TOKENS_METRIC = "prompt_tokens_total"
PREFILL_COMPUTED_METRIC = "request_prefill_kv_computed_tokens_sum"
KV_STORED_METRIC = "kv_stored"


def normalize_summary_name(raw_name):
	raw_name = raw_name.strip().lower()
	raw_name = raw_name.replace("%", "pct")
	raw_name = raw_name.replace(" ", "_")
	raw_name = re.sub(r"[^a-z0-9_]+", "", raw_name)
	return raw_name


def parse_log_metrics(log_file_path):
	requests = []
	current_request = None
	in_summary = False
	summary_metrics = {}

	with log_file_path.open("r", encoding="utf-8", errors="replace") as handle:
		for raw_line in handle:
			line = raw_line.strip()
			if not line:
				continue

			if SUMMARY_START_RE.match(line):
				if current_request is not None:
					requests.append(current_request)
					current_request = None
				in_summary = True
				continue

			if in_summary and SUMMARY_END_RE.match(line):
				in_summary = False
				continue

			if RUNNER_LINE_RE.match(line):
				if current_request is not None:
					requests.append(current_request)
				current_request = {}
				continue

			metric_match = METRIC_LINE_RE.match(line)
			if metric_match and current_request is not None and not in_summary:
				metric_name = metric_match.group("name")
				metric_value = float(metric_match.group("value"))
				current_request[metric_name] = metric_value
				continue

			if in_summary:
				if metric_match:
					summary_metrics[metric_match.group("name")] = float(metric_match.group("value"))
					continue

				summary_match = SUMMARY_NUMBER_RE.match(line)
				if summary_match:
					raw_name = summary_match.group("name")
					summary_name = normalize_summary_name(raw_name)
					summary_metrics[summary_name] = float(summary_match.group("value"))

	if current_request is not None:
		requests.append(current_request)

	for request in requests:
		if PROMPT_TOKENS_METRIC in request and PREFILL_COMPUTED_METRIC in request:
			request[KV_STORED_METRIC] = (
				request[PROMPT_TOKENS_METRIC] - request[PREFILL_COMPUTED_METRIC]
			)

	if PROMPT_TOKENS_METRIC in summary_metrics and PREFILL_COMPUTED_METRIC in summary_metrics:
		summary_metrics[KV_STORED_METRIC] = (
			summary_metrics[PROMPT_TOKENS_METRIC]
			- summary_metrics[PREFILL_COMPUTED_METRIC]
		)

	return requests, summary_metrics


def discover_logs_by_name(models_root, model_glob):
	grouped_logs = defaultdict(dict)

	for model_dir in sorted(models_root.glob(model_glob)):
		if not model_dir.is_dir():
			continue

		model_name = model_dir.name
		for log_file in sorted(model_dir.glob("*.txt")):
			grouped_logs[log_file.name][model_name] = log_file

	return grouped_logs


def collect_metric_series(requests):
	metric_names = sorted({metric for request in requests for metric in request})
	series_by_metric = {}

	for metric_name in metric_names:
		series_by_metric[metric_name] = [
			request.get(metric_name, math.nan) for request in requests
		]

	return series_by_metric


def sanitize_filename(value):
	return re.sub(r"[^A-Za-z0-9_.-]", "_", value)


def plot_metric_for_log(log_name, metric_name, model_series, output_dir, show_plots):
	plt.figure(figsize=(12, 6))
	line_styles = cycle(["-", "--", "-.", ":"])
	markers = cycle(["o", "s", "^", "D", "v", "P", "X"])
	model_items = sorted(model_series.items())
	model_count = len(model_items)
	max_points = max((len(series) for _, series in model_items), default=0)

	# Add a tiny horizontal offset per model so identical lines do not fully overlap.
	if model_count > 1:
		offset_step = 0.22 / (model_count - 1)
	else:
		offset_step = 0.0

	for idx, (model_name, series) in enumerate(model_items):
		if not series:
			continue
		offset = -0.11 + idx * offset_step if model_count > 1 else 0.0
		x_values = [x + offset for x in range(1, len(series) + 1)]
		plt.plot(
			x_values,
			series,
			marker=next(markers),
			linestyle=next(line_styles),
			markersize=3.5,
			linewidth=1.7,
			alpha=0.9,
			label=model_name,
		)

	if max_points > 0:
		plt.xticks(list(range(1, max_points + 1)))

	plt.title(f"{log_name} | {metric_name}")
	plt.xlabel("Request Number")
	plt.ylabel("Metric Value")
	plt.legend(ncol=2)
	plt.grid(True, alpha=0.3)
	plt.tight_layout()

	file_name = f"{sanitize_filename(Path(log_name).stem)}__{sanitize_filename(metric_name)}.png"
	output_path = output_dir / file_name
	plt.savefig(output_path, dpi=160)
	print(f"Saved plot: {output_path}")

	if show_plots:
		plt.show()
	else:
		plt.close()


def plot_summary_metric_for_log(log_name, metric_name, model_values, output_dir, show_plots):
	plt.figure(figsize=(10, 5.5))

	model_names = sorted(model_values)
	y_values = [model_values[name] for name in model_names]
	x_values = list(range(1, len(model_names) + 1))

	plt.plot(x_values, y_values, marker="o", linewidth=1.8)
	plt.xticks(x_values, model_names, rotation=25, ha="right")
	plt.title(f"Summary | {log_name} | {metric_name}")
	plt.xlabel("Model")
	plt.ylabel("Summary Value")
	plt.grid(True, alpha=0.3)
	plt.tight_layout()

	file_name = (
		f"{sanitize_filename(Path(log_name).stem)}__summary__{sanitize_filename(metric_name)}.png"
	)
	output_path = output_dir / file_name
	plt.savefig(output_path, dpi=160)
	print(f"Saved summary plot: {output_path}")

	if show_plots:
		plt.show()
	else:
		plt.close()


def build_plots(models_root, model_glob, output_dir, show_plots):
	grouped_logs = discover_logs_by_name(models_root, model_glob)
	if not grouped_logs:
		raise SystemExit(
			f"No model directories or log files found under {models_root} with glob {model_glob}."
		)

	output_dir.mkdir(parents=True, exist_ok=True)

	for log_name, model_to_file in sorted(grouped_logs.items()):
		model_to_metric_series = {}
		model_to_summary_metrics = {}
		all_metric_names = set()
		all_summary_names = set()

		for model_name, log_path in sorted(model_to_file.items()):
			requests, summary_metrics = parse_log_metrics(log_path)
			metric_series = collect_metric_series(requests)
			model_to_metric_series[model_name] = metric_series
			model_to_summary_metrics[model_name] = summary_metrics
			all_metric_names.update(metric_series.keys())
			all_summary_names.update(summary_metrics.keys())
			print(f"Parsed {log_path}: {len(requests)} request(s)")

		if not all_metric_names:
			print(f"Skipping {log_name}: no per-request vllm metrics found.")
			continue

		print(f"Plotting {len(all_metric_names)} metric(s) for {log_name}.")
		for metric_name in sorted(all_metric_names):
			model_series = {
				model_name: model_to_metric_series[model_name].get(metric_name, [])
				for model_name in model_to_metric_series
			}
			plot_metric_for_log(log_name, metric_name, model_series, output_dir, show_plots)

		if all_summary_names:
			print(f"Plotting {len(all_summary_names)} summary metric(s) for {log_name}.")
			for metric_name in sorted(all_summary_names):
				model_values = {
					model_name: model_to_summary_metrics[model_name][metric_name]
					for model_name in model_to_summary_metrics
					if metric_name in model_to_summary_metrics[model_name]
				}
				if model_values:
					plot_summary_metric_for_log(
						log_name, metric_name, model_values, output_dir, show_plots
					)


def parse_args():
	parser = argparse.ArgumentParser(
		description=(
			"Parse vLLM benchmark logs from model folders and create one line plot per metric. "
			"Logs with the same filename are plotted together across models."
		)
	)
	parser.add_argument(
		"models_dir",
		nargs="?",
		default="logs",
		help="Root directory containing model subdirectories (default: logs)",
	)
	parser.add_argument(
		"--model-glob",
		default="opt-*",
		help="Glob used to select model directories under models_dir (default: opt-*)",
	)
	parser.add_argument(
		"--output-dir",
		default="plots/vllm_metrics",
		help="Directory for output plot images (default: plots/vllm_metrics)",
	)
	parser.add_argument(
		"--show",
		action="store_true",
		help="Display plots in a window in addition to saving images",
	)
	return parser.parse_args()


def main():
	args = parse_args()
	models_root = Path(args.models_dir)
	output_dir = Path(args.output_dir)

	if not models_root.exists() or not models_root.is_dir():
		raise SystemExit(f"models_dir does not exist or is not a directory: {models_root}")

	build_plots(models_root, args.model_glob, output_dir, args.show)


if __name__ == "__main__":
	main()
