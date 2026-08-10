"""
蝉镜 API 调试脚本 —— 逐字段排查 400 错误
用法: python debug_chanjing_api.py
"""

import requests
import json
import os
import sys
from typing import Any

# ============================================================
# 配置区 —— 请填入你的蝉镜 API 凭据
# ============================================================
APP_ID = "44602067"
SECRET_KEY = os.environ.get("CHANJING_SK", "你的SecretKey")  # 或用环境变量

BASE_URL = "https://open-api.chanjing.cc/open/v1"

# 从 Go 代码中取出的默认参数
DEFAULT_VIDEO_FILE_ID = "C-08d3a2d36b764b59bbff0349338e5639"
DEFAULT_AUDIO_MAN_ID = "C-06d6082c2f39471b8cd28a2f1585b170"

# ============================================================
# Token
# ============================================================
token_cache = {"token": None, "expiry": 0}


def get_token() -> str:
    import time

    if token_cache["token"] and time.time() < token_cache["expiry"]:
        return token_cache["token"]

    resp = requests.post(
        f"{BASE_URL}/access_token",
        json={"app_id": APP_ID, "secret_key": SECRET_KEY},
        timeout=10,
    )
    data = resp.json()
    print(f"[TOKEN] resp: {json.dumps(data, ensure_ascii=False)}")
    if data.get("code") != 0:
        raise RuntimeError(f"获取 token 失败: {data}")

    token_cache["token"] = data["data"]["access_token"]
    token_cache["expiry"] = time.time() + 3000  # 50 min
    return token_cache["token"]


# ============================================================
# 请求构建
# ============================================================


def create_video(payload: dict) -> dict:
    token = get_token()
    print(f"\n{'='*60}")
    print(f"请求体: {json.dumps(payload, ensure_ascii=False, indent=2)}")
    print(f"{'='*60}")

    resp = requests.post(
        f"{BASE_URL}/video_lip_sync/create",
        json=payload,
        headers={
            "Content-Type": "application/json",
            "access_token": token,
        },
        timeout=30,
    )
    data = resp.json()
    print(f"HTTP {resp.status_code} → {json.dumps(data, ensure_ascii=False, indent=2)}")

    if data.get("code") != 0:
        print(f"❌ 失败: code={data.get('code')}, msg={data.get('msg')}")
    else:
        print(f"✅ 成功: task_id={data.get('data')}")
    return data


# ============================================================
# 测试用例
# ============================================================

# 1. 基准：和 Go 代码完全一致的请求
BASELINE = {
    "video_file_id": DEFAULT_VIDEO_FILE_ID,
    "screen_width": 1080,
    "screen_height": 1920,
    "model": 0,
    "audio_type": "tts",
    "tts_config": {
        "text": "测试稿子123",
        "audio_man_id": DEFAULT_AUDIO_MAN_ID,
        "speed": 1.0,
    },
}


def test_baseline():
    """测试1: 基准——完全复刻 Go 代码的请求"""
    print("\n🔍 测试1: 基准请求（复刻 Go 代码）")
    return create_video(BASELINE)


def test_without_model():
    """测试2: 去掉 model 字段"""
    print("\n🔍 测试2: 去掉 model 字段")
    p = {k: v for k, v in BASELINE.items() if k != "model"}
    return create_video(p)


def test_without_screen():
    """测试3: 去掉 screen_width/screen_height"""
    print("\n🔍 测试3: 去掉 screen_width 和 screen_height")
    p = {k: v for k, v in BASELINE.items() if k not in ("screen_width", "screen_height")}
    return create_video(p)


def test_videofile_no_prefix():
    """测试4: video_file_id 去掉 C- 前缀"""
    print("\n🔍 测试4: video_file_id 去掉 C- 前缀")
    p = dict(BASELINE)
    p["video_file_id"] = p["video_file_id"].replace("C-", "")
    return create_video(p)


def test_minimal_fields():
    """测试5: 最小必填字段（只保留 video_file_id, audio_type, tts_config）"""
    print("\n🔍 测试5: 最小必填字段")
    p = {
        "video_file_id": DEFAULT_VIDEO_FILE_ID,
        "audio_type": "tts",
        "tts_config": {
            "text": "测试123",
            "audio_man_id": DEFAULT_AUDIO_MAN_ID,
        },
    }
    return create_video(p)


def test_speed_int():
    """测试6: speed 用整数 1 而非 1.0"""
    print("\n🔍 测试6: speed 用整数 1")
    p = dict(BASELINE)
    import copy
    p["tts_config"] = copy.deepcopy(BASELINE["tts_config"])
    p["tts_config"]["speed"] = 1  # int, not float
    return create_video(p)


def test_with_pitch():
    """测试7: tts_config 加上 pitch 字段"""
    print("\n🔍 测试7: tts_config 加 pitch: 1")
    p = dict(BASELINE)
    import copy
    p["tts_config"] = copy.deepcopy(BASELINE["tts_config"])
    p["tts_config"]["pitch"] = 1
    return create_video(p)


def test_with_backway_drivemode():
    """测试8: 加上 backway 和 drive_mode 默认值"""
    print("\n🔍 测试8: 加 backway:1, drive_mode:''")
    p = dict(BASELINE)
    p["backway"] = 1
    p["drive_mode"] = ""
    return create_video(p)


def test_api_doc_example():
    """测试9: 完全按 API 文档示例格式（只用文档里的字段）"""
    print("\n🔍 测试9: 完全按 API 文档示例")
    p = {
        "video_file_id": DEFAULT_VIDEO_FILE_ID,
        "screen_width": 1080,
        "screen_height": 1920,
        "model": 0,
        "audio_type": "tts",
        "tts_config": {
            "text": "君不见黄河之水天上来，奔流到海不复回。",
            "audio_man_id": DEFAULT_AUDIO_MAN_ID,
            "speed": 1,
            "pitch": 1,
        },
    }
    return create_video(p)


def test_different_video_file():
    """测试10: 用不同的 video_file_id（列表中的另一个形象）"""
    print("\n🔍 测试10: 换一个 video_file_id")
    p = dict(BASELINE)
    p["video_file_id"] = "C-6a951f1434884368bc4942f7f74885ff"
    return create_video(p)


def test_content_type_variation():
    """测试11: 尝试用 data= 而非 json= 发请求（控制 Content-Type 精确格式）"""
    print("\n🔍 测试11: 手动控制 Content-Type 和 body 编码")
    token = get_token()
    payload_str = json.dumps(BASELINE, ensure_ascii=False)
    resp = requests.post(
        f"{BASE_URL}/video_lip_sync/create",
        data=payload_str.encode("utf-8"),
        headers={
            "Content-Type": "application/json; charset=utf-8",
            "access_token": token,
        },
        timeout=30,
    )
    data = resp.json()
    print(f"HTTP {resp.status_code} → {json.dumps(data, ensure_ascii=False, indent=2)}")
    return data


# ============================================================
# 执行
# ============================================================
ALL_TESTS = [
    ("基准请求（复刻Go）", test_baseline),
    ("去掉model", test_without_model),
    ("去掉screen尺寸", test_without_screen),
    ("video_file_id去C-前缀", test_videofile_no_prefix),
    ("最小必填字段", test_minimal_fields),
    ("speed用整数", test_speed_int),
    ("加pitch字段", test_with_pitch),
    ("加backway+drive_mode", test_with_backway_drivemode),
    ("按API文档示例", test_api_doc_example),
    ("换一个video_file_id", test_different_video_file),
    ("手动Content-Type", test_content_type_variation),
]


def main():
    if SECRET_KEY == "你的SecretKey":
        print("❌ 请先设置 CHANJING_SK 环境变量，或直接修改脚本里的 SECRET_KEY")
        print("   export CHANJING_SK=你的蝉镜SecretKey")
        sys.exit(1)

    results = []
    for name, fn in ALL_TESTS:
        try:
            data = fn()
            ok = data.get("code") == 0
            results.append((name, ok, data.get("msg", "")))
            if ok:
                print(f"\n🎉 成功！{name} 返回了有效响应。停止后续测试。")
                break
        except Exception as e:
            print(f"💥 异常: {e}")
            results.append((name, False, str(e)))

    # 汇总
    print(f"\n{'='*60}")
    print("汇总:")
    for name, ok, detail in results:
        status = "✅" if ok else "❌"
        print(f"  {status} {name}" + (f" — {detail}" if detail else ""))

    # 特别检查：如果某个去掉 video_file_id C- 前缀的测试通过
    # 说明问题就在这里
    for name, ok, detail in results:
        if ok and "C-前缀" in name:
            print("\n⚠️  根因定位: video_file_id 不应该带 C- 前缀！需要修改 seed.go 去掉 C- 前缀。")
        if ok and "最小必填" in name:
            print("\n⚠️  根因定位: 多余字段导致 400。需要逐一加回字段找出是哪个。")


if __name__ == "__main__":
    main()
