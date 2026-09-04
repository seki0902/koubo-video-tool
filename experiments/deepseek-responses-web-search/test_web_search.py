#!/usr/bin/env python3
"""Independent DeepSeek Responses API web_search quality probe.

This program intentionally has no imports from the main Koubo application. It
uses only Python's standard library so the request body and the complete raw
Responses API response remain easy to audit.
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import re
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlparse
from urllib.request import Request, build_opener


QUERIES = [
    "香港外企招聘 2026 2027届",
    "香港中资企业校园招聘",
    "香港金融机构应届生招聘",
    "香港咨询公司校招",
    "香港互联网公司招聘毕业生",
    "江浙沪外企招聘会 2026",
    "香港人才引进计划 2026 毕业生",
]


PROMPTS = {
    "basic": """请使用内置 web_search 搜索以下主题：
「{query}」

然后基于搜索结果，只输出一个 JSON 对象，格式如下：

{
  "results": [
    {
      "title": "结果标题，必须来自搜索结果原文",
      "url": "结果链接，必须来自搜索结果原文",
      "snippet": "不超过100字的摘要，必须来自搜索结果原文",
      "published_date": "如果搜索结果中有明确时间则填写，否则填 null"
    }
  ],
  "total_found": 结果数量,
  "search_executed": true
}

要求：
- 每个字段都必须来自搜索结果，不得编造或根据常识补充
- 如果某条结果没有 url 或 title，就丢弃它，不要硬填
- 如果搜索结果不足以回答，返回 {"results": [], "total_found": 0, "search_executed": true}
- 不要输出任何 JSON 以外的解释文字""",
    "strict": """请使用内置 web_search 搜索：
「{query}」

你的任务是找出真实存在的、面向应届生或在校生的招聘信息/招聘活动/招聘会/企业校招页面。

只输出一个 JSON 数组，每个元素格式如下：

{
  "company_or_event": "公司名或活动名，必须来自搜索结果",
  "url": "原始链接",
  "target_audience": "招聘对象，如2026届/2027届/应届生/不限，没有则填 null",
  "application_deadline": "申请截止日期，没有则填 null",
  "location": "工作地点或活动地点，没有则填 null",
  "source_type": "official_job_page / job_aggregator / news / event_page / other"
}

过滤规则：
- 只保留与“招聘”“校招”“应届生”“实习”“招聘会”直接相关的结果
- 百科、旅游、行业新闻、政策解读等不相关的结果一律丢弃
- 如果某个字段在搜索结果中缺失，就填 null，禁止脑补
- 如果没有任何符合条件的结果，返回空数组 []
- 不要输出解释，只输出 JSON""",
    "extreme": """请使用内置 web_search 搜索：
「{query}」

然后基于搜索返回的内容，输出 JSON。

要求：
- 只输出一个 JSON 对象，不要输出 JSON 以外的文字
- 字段包括：query, search_success, results
- results 数组中的每一项必须同时包含 title 和 url，这两个字段必须逐字来自搜索结果
- 如果搜索结果为空、被截断、没有返回任何 url，search_success 设为 false，results 设为 []
- 如果搜索结果中没有任何一个 url 可以证明是真实存在的招聘页面，search_success 设为 false，results 设为 []
- 绝对禁止根据模型自身知识生成 title、url 或 company 名称
- 绝对禁止把培训广告、泛招聘平台首页当作有效结果
- 只要有一条结果符合要求，search_success 就为 true，results 只包含符合要求的结果

输出示例：
{"query":"{query}","search_success":true,"results":[{"title":"...","url":"..."}]}""",
}

PROMPT_LEVELS = tuple(PROMPTS)
SOURCE_TYPES = {"official_job_page", "job_aggregator", "news", "event_page", "other"}


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def contains_web_search_call(value: Any) -> bool:
    if isinstance(value, dict):
        if value.get("type") == "web_search_call":
            return True
        return any(contains_web_search_call(item) for item in value.values())
    if isinstance(value, list):
        return any(contains_web_search_call(item) for item in value)
    return False


def collect_messages(response: dict[str, Any]) -> list[dict[str, Any]]:
    return [
        item
        for item in response.get("output", [])
        if isinstance(item, dict) and item.get("type") == "message"
    ]


def message_text(messages: list[dict[str, Any]]) -> str:
    chunks: list[str] = []
    for message in messages:
        content = message.get("content", [])
        if isinstance(content, str):
            chunks.append(content)
        elif isinstance(content, list):
            for part in content:
                if isinstance(part, dict) and part.get("type") == "output_text":
                    chunks.append(str(part.get("text", "")))
    return "\n".join(chunk for chunk in chunks if chunk)


def parse_structured_message(text: str) -> tuple[Any, str | None]:
    if not text.strip():
        return None, "empty assistant message"
    try:
        return json.loads(text), None
    except json.JSONDecodeError as exc:
        return None, f"invalid JSON: {exc}"


def prompt_for(level: str, query: str) -> str:
    try:
        template = PROMPTS[level]
    except KeyError as exc:
        raise ValueError(f"unknown prompt level: {level}") from exc
    return template.replace("{query}", query)


def is_http_url(value: Any) -> bool:
    if not isinstance(value, str):
        return False
    parsed = urlparse(value)
    return parsed.scheme in {"http", "https"} and bool(parsed.netloc)


def validate_contract(value: Any, level: str, query: str) -> dict[str, Any]:
    """Check only machine-verifiable shape rules from the selected prompt.

    Whether a title/snippet really came from a search result still requires
    comparing the model message with the raw web_search evidence. This helper
    deliberately does not claim to prove provenance.
    """
    errors: list[str] = []
    if level == "basic":
        if not isinstance(value, dict):
            errors.append("top-level value is not an object")
            return {"valid": False, "errors": errors}
        results = value.get("results")
        if not isinstance(results, list):
            errors.append("results is not an array")
            results = []
        if value.get("search_executed") is not True:
            errors.append("search_executed is not true")
        if not isinstance(value.get("total_found"), int) or isinstance(value.get("total_found"), bool):
            errors.append("total_found is not an integer")
        elif value["total_found"] != len(results):
            errors.append("total_found does not equal results length")
        for index, item in enumerate(results):
            if not isinstance(item, dict):
                errors.append(f"results[{index}] is not an object")
                continue
            for field in ("title", "url", "snippet"):
                if not isinstance(item.get(field), str) or not item[field].strip():
                    errors.append(f"results[{index}].{field} is missing")
            if isinstance(item.get("snippet"), str) and len(item["snippet"]) > 100:
                errors.append(f"results[{index}].snippet exceeds 100 characters")
            if not (item.get("published_date") is None or isinstance(item.get("published_date"), str)):
                errors.append(f"results[{index}].published_date is not string or null")
    elif level == "strict":
        if not isinstance(value, list):
            errors.append("top-level value is not an array")
            return {"valid": False, "errors": errors}
        required = ("company_or_event", "url", "target_audience", "application_deadline", "location", "source_type")
        for index, item in enumerate(value):
            if not isinstance(item, dict):
                errors.append(f"results[{index}] is not an object")
                continue
            if not isinstance(item.get("company_or_event"), str) or not item["company_or_event"].strip():
                errors.append(f"results[{index}].company_or_event is missing")
            if not is_http_url(item.get("url")):
                errors.append(f"results[{index}].url is not a valid HTTP URL")
            for field in required[2:5]:
                if not (item.get(field) is None or isinstance(item.get(field), str)):
                    errors.append(f"results[{index}].{field} is not string or null")
            if item.get("source_type") not in SOURCE_TYPES:
                errors.append(f"results[{index}].source_type is invalid")
    elif level == "extreme":
        if not isinstance(value, dict):
            errors.append("top-level value is not an object")
            return {"valid": False, "errors": errors}
        if value.get("query") != query:
            errors.append("query does not equal the requested query")
        if not isinstance(value.get("search_success"), bool):
            errors.append("search_success is not boolean")
        results = value.get("results")
        if not isinstance(results, list):
            errors.append("results is not an array")
            results = []
        if value.get("search_success") is False and results:
            errors.append("search_success=false but results is not empty")
        if value.get("search_success") is True and not results:
            errors.append("search_success=true but results is empty")
        for index, item in enumerate(results):
            if not isinstance(item, dict):
                errors.append(f"results[{index}] is not an object")
                continue
            for field in ("title", "url"):
                if not isinstance(item.get(field), str) or not item[field].strip():
                    errors.append(f"results[{index}].{field} is missing")
            if not is_http_url(item.get("url")):
                errors.append(f"results[{index}].url is not a valid HTTP URL")
    else:
        return {"valid": None, "errors": [f"unknown prompt level: {level}"]}
    return {"valid": not errors, "errors": errors}


def declared_search_flags(value: Any, level: str) -> dict[str, Any]:
    """Expose the search status fields requested by the prompt, when present."""
    if not isinstance(value, dict):
        return {"declared_search_executed": None, "declared_search_success": None}
    return {
        "declared_search_executed": value.get("search_executed") if level == "basic" else None,
        "declared_search_success": value.get("search_success") if level == "extreme" else None,
    }


def collect_urls(value: Any, key: str = "") -> list[str]:
    urls: list[str] = []
    if isinstance(value, dict):
        for child_key, child_value in value.items():
            urls.extend(collect_urls(child_value, child_key.lower()))
    elif isinstance(value, list):
        for child in value:
            urls.extend(collect_urls(child, key))
    elif isinstance(value, str):
        for match in re.findall(r"https?://[^\s,，；;）)\]】]+", value):
            urls.append(match.rstrip(".。,'\""))
    return urls


def check_url(url: str, timeout: float) -> dict[str, Any]:
    parsed = urlparse(url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return {"url": url, "accessible": False, "status": None, "error": "invalid URL"}

    opener = build_opener()
    headers = {
        "User-Agent": "koubo-deepseek-web-search-probe/1.0",
        "Accept": "text/html,application/xhtml+xml,*/*;q=0.1",
    }
    try:
        request = Request(url, headers=headers, method="HEAD")
        with opener.open(request, timeout=timeout) as response:
            status = response.status
            return {"url": url, "accessible": 200 <= status < 400, "status": status}
    except HTTPError as exc:
        # Some sites reject HEAD but still serve a normal GET.
        if exc.code not in {400, 403, 405, 501}:
            return {"url": url, "accessible": 200 <= exc.code < 400, "status": exc.code}
    except (URLError, TimeoutError, OSError, UnicodeError, ValueError):
        pass

    try:
        request = Request(url, headers={**headers, "Range": "bytes=0-1023"}, method="GET")
        with opener.open(request, timeout=timeout) as response:
            status = response.status
            response.read(1024)
            return {"url": url, "accessible": 200 <= status < 400, "status": status}
    except HTTPError as exc:
        return {"url": url, "accessible": 200 <= exc.code < 400, "status": exc.code}
    except (URLError, TimeoutError, OSError, UnicodeError, ValueError) as exc:
        return {"url": url, "accessible": False, "status": None, "error": str(exc)}


def request_response(api_key: str, model: str, query: str, prompt_level: str, timeout: float) -> tuple[Any, str | None, int]:
    payload = {
        "model": model,
        "input": prompt_for(prompt_level, query),
        "tools": [{"type": "web_search"}],
        "tool_choice": {"type": "web_search"},
    }
    # The strict prompt intentionally asks for a top-level JSON array. Keep
    # the API's object-only JSON helper for the two object-shaped prompts, but
    # let the strict case test whether the model follows its own instruction.
    if prompt_level != "strict":
        payload["text"] = {"format": {"type": "json_object"}}
    request = Request(
        "https://api.deepseek.com/responses",
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
        method="POST",
    )
    started = time.perf_counter()
    try:
        with build_opener().open(request, timeout=timeout) as response:
            body = response.read()
            elapsed = round((time.perf_counter() - started) * 1000)
            return json.loads(body.decode("utf-8")), None, elapsed
    except HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        try:
            raw: Any = json.loads(body)
        except json.JSONDecodeError:
            raw = {"_raw_text": body}
        elapsed = round((time.perf_counter() - started) * 1000)
        return raw, f"HTTP {exc.code}", elapsed
    except (URLError, TimeoutError, OSError) as exc:
        elapsed = round((time.perf_counter() - started) * 1000)
        return {"_error": str(exc)}, str(exc), elapsed


def safe_slug(value: str) -> str:
    slug = re.sub(r"[^0-9A-Za-z\u4e00-\u9fff]+", "_", value).strip("_")
    return slug[:60] or "query"


def run_one(api_key: str, model: str, query: str, prompt_level: str, index: int, output_dir: Path, url_timeout: float, api_timeout: float) -> dict[str, Any]:
    input_prompt = prompt_for(prompt_level, query)
    raw_response, request_error, duration_ms = request_response(api_key, model, query, prompt_level, api_timeout)
    response_object = raw_response if isinstance(raw_response, dict) else {"value": raw_response}
    messages = collect_messages(response_object)
    final_text = message_text(messages)
    structured, parse_error = parse_structured_message(final_text)
    contract = validate_contract(structured, prompt_level, query) if parse_error is None else {"valid": False, "errors": [parse_error]}
    search_flags = declared_search_flags(structured, prompt_level)
    urls = sorted(set(collect_urls(structured))) if parse_error is None else []
    url_checks = [check_url(url, url_timeout) for url in urls]

    usage = response_object.get("usage", {}) if isinstance(response_object, dict) else {}
    result_count: int | None = None
    if isinstance(structured, dict):
        for result_key in (
            "results", "recruitments", "recruitment_listings", "recruitment_events",
            "platforms_and_events", "job_fairs", "items", "jobs", "opportunities", "positions",
        ):
            if isinstance(structured.get(result_key), list):
                result_count = len(structured[result_key])
                break
    elif isinstance(structured, list):
        result_count = len(structured)

    record = {
        "probe": {
            "query": query,
            "model": model,
            "prompt_level": prompt_level,
            "input_prompt": input_prompt,
            "started_at": now_iso(),
            "duration_ms": duration_ms,
            "request_error": request_error,
            "web_search_call": contains_web_search_call(response_object),
            "web_search_call_count": count_type(response_object, "web_search_call"),
            "assistant_message_count": len(messages),
            "final_structured_message": structured,
            "final_message_text": final_text,
            "json_parse_error": parse_error,
            "contract_valid": contract["valid"],
            "contract_errors": contract["errors"],
            **search_flags,
            "result_count": result_count,
            "urls": urls,
            "url_checks": url_checks,
            "url_accessible_count": sum(1 for item in url_checks if item.get("accessible")),
            "usage": usage,
        },
        # Preserve the full API object exactly as decoded, including output
        # web_search_call/reasoning/message items and all usage fields.
        "raw_response": raw_response,
    }
    filename = f"{index:02d}_{prompt_level}_{model}_{safe_slug(query)}.json"
    (output_dir / filename).write_text(
        json.dumps(record, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    return record["probe"]


def refresh_one(path: Path, url_timeout: float) -> dict[str, Any]:
    """Recompute derived metrics without making another API request."""
    record = json.loads(path.read_text(encoding="utf-8"))
    raw_response = record.get("raw_response", {})
    response_object = raw_response if isinstance(raw_response, dict) else {"value": raw_response}
    messages = collect_messages(response_object)
    final_text = message_text(messages)
    structured, parse_error = parse_structured_message(final_text)
    prompt_level = record.get("probe", {}).get("prompt_level", "legacy")
    query = record.get("probe", {}).get("query", "")
    if parse_error is None and prompt_level in PROMPTS:
        contract = validate_contract(structured, prompt_level, query)
    else:
        contract = {"valid": None, "errors": [] if parse_error is None else [parse_error]}
    search_flags = declared_search_flags(structured, prompt_level)
    urls = sorted(set(collect_urls(structured))) if parse_error is None else []
    url_checks = [check_url(url, url_timeout) for url in urls]
    result_count: int | None = None
    if isinstance(structured, dict):
        for result_key in (
            "results", "recruitments", "recruitment_listings", "recruitment_events",
            "platforms_and_events", "job_fairs", "items", "jobs", "opportunities", "positions",
        ):
            if isinstance(structured.get(result_key), list):
                result_count = len(structured[result_key])
                break
    elif isinstance(structured, list):
        result_count = len(structured)
    probe = record["probe"]
    probe.update({
        "prompt_level": prompt_level,
        "web_search_call": contains_web_search_call(response_object),
        "web_search_call_count": count_type(response_object, "web_search_call"),
        "assistant_message_count": len(messages),
        "final_structured_message": structured,
        "final_message_text": final_text,
        "json_parse_error": parse_error,
        "contract_valid": contract["valid"],
        "contract_errors": contract["errors"],
        **search_flags,
        "result_count": result_count,
        "urls": urls,
        "url_checks": url_checks,
        "url_accessible_count": sum(1 for item in url_checks if item.get("accessible")),
        "usage": response_object.get("usage", {}),
    })
    path.write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return probe


def count_type(value: Any, expected: str) -> int:
    if isinstance(value, dict):
        return (1 if value.get("type") == expected else 0) + sum(count_type(item, expected) for item in value.values())
    if isinstance(value, list):
        return sum(count_type(item, expected) for item in value)
    return 0


def write_summaries(rows: list[dict[str, Any]], output_dir: Path) -> None:
    fields = [
        "model", "prompt_level", "query", "duration_ms", "input_tokens", "output_tokens",
        "reasoning_tokens", "total_tokens", "web_search_call", "web_search_call_count",
        "json_parse_error", "contract_valid", "contract_errors", "declared_search_executed", "declared_search_success",
        "result_count", "url_count", "url_accessible_count",
        "request_error",
    ]
    summary_rows: list[dict[str, Any]] = []
    for row in rows:
        usage = row.get("usage") or {}
        summary_rows.append({
            "model": row["model"],
            "prompt_level": row.get("prompt_level", "legacy"),
            "query": row["query"],
            "duration_ms": row["duration_ms"],
            "input_tokens": usage.get("input_tokens"),
            "output_tokens": usage.get("output_tokens"),
            "reasoning_tokens": (usage.get("output_tokens_details") or {}).get("reasoning_tokens"),
            "total_tokens": usage.get("total_tokens"),
            "web_search_call": row["web_search_call"],
            "web_search_call_count": row["web_search_call_count"],
            "json_parse_error": row["json_parse_error"],
            "contract_valid": row.get("contract_valid"),
            "contract_errors": " | ".join(row.get("contract_errors") or []),
            "declared_search_executed": row.get("declared_search_executed"),
            "declared_search_success": row.get("declared_search_success"),
            "result_count": row["result_count"],
            "url_count": len(row["urls"]),
            "url_accessible_count": row["url_accessible_count"],
            "request_error": row["request_error"],
        })
    with (output_dir / "summary.csv").open("w", newline="", encoding="utf-8-sig") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields)
        writer.writeheader()
        writer.writerows(summary_rows)
    (output_dir / "summary.json").write_text(
        json.dumps({"generated_at": now_iso(), "rows": summary_rows}, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="DeepSeek Responses API web_search probe")
    parser.add_argument("--output-dir", type=Path, default=Path("artifacts/deepseek-responses-web-search"))
    parser.add_argument("--models", default="deepseek-v4-flash", help="comma-separated models; use flash,pro for a comparison")
    parser.add_argument(
        "--prompt-levels",
        default=",".join(PROMPT_LEVELS),
        help="comma-separated prompt levels: basic,strict,extreme",
    )
    parser.add_argument("--api-timeout", type=float, default=180.0)
    parser.add_argument("--url-timeout", type=float, default=10.0)
    parser.add_argument("--refresh-dir", type=Path, help="refresh derived metrics from saved raw files without API calls")
    args = parser.parse_args()

    if args.refresh_dir:
        files = sorted(path for path in args.refresh_dir.glob("*.json") if path.name != "summary.json")
        rows = [refresh_one(path, args.url_timeout) for path in files]
        write_summaries(rows, args.refresh_dir)
        print(f"Refreshed {len(rows)} saved response files and summary files in {args.refresh_dir}")
        return 0

    api_key = os.environ.get("DEEPSEEK_API_KEY", "").strip()
    if not api_key:
        print("DEEPSEEK_API_KEY is not set", file=sys.stderr)
        return 2
    models = [item.strip() for item in args.models.split(",") if item.strip()]
    prompt_levels = [item.strip() for item in args.prompt_levels.split(",") if item.strip()]
    unknown_levels = [item for item in prompt_levels if item not in PROMPTS]
    if not prompt_levels or unknown_levels:
        choices = ", ".join(PROMPT_LEVELS)
        parser.error(f"--prompt-levels must contain only {choices}; got {unknown_levels or 'empty'}")
    args.output_dir.mkdir(parents=True, exist_ok=True)
    rows: list[dict[str, Any]] = []
    for model in models:
        for index, query in enumerate(QUERIES, 1):
            for prompt_level in prompt_levels:
                print(f"[{model}/{prompt_level}] {index}/{len(QUERIES)} {query}", flush=True)
                row = run_one(api_key, model, query, prompt_level, index, args.output_dir, args.url_timeout, args.api_timeout)
                rows.append(row)
    write_summaries(rows, args.output_dir)
    print(f"Saved {len(rows)} raw response files plus summary.csv/summary.json to {args.output_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
