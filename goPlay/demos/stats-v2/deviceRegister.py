import time
import uuid
import random
import json
import requests
import string
from urllib.parse import urlencode
from SignerPy import sign

DEVICE_POOL = [
    ("SM-F936B", "samsung", "904*2105", 420),
    ("M2012K11AG", "xiaomi", "904*2105", 440),
    ("RMX3081", "realme", "904*2105", 480),
    ("Pixel 6", "google", "904*2105", 420),
    ("CPH2411", "oppo", "904*2105", 440),
]

def _digits(n=19):
    return "".join(random.choices(string.digits, k=n))

def _hex(n=16):
    return "".join(random.choices("0123456789abcdef", k=n))

def get_density_class(dpi):
    if dpi < 400:
        return "hdpi"
    if dpi < 440:
        return "xhdpi"
    if dpi < 480:
        return "xxhdpi"
    return "xxxhdpi"

# def send_app_log(device):
#     ts = int(time.time())
#     rticket = ts * 1000

#     params = {
#         "device_platform": "android",
#         "os": "android",
#         "ssmix": "a",
#         "_rticket": rticket,
#         "cdid": device["cdid"],
#         "channel": "googleplay",
#         "aid": "1233",
#         "app_name": "musical_ly",
#         "version_code": "350003",
#         "version_name": "35.0.3",
#         "device_id": device["device_id"],
#         "iid": device["install_id"],
#         "ts": ts,
#         "region": "TR",
#         "language": "tr",
#         "host_abi": "arm64-v8a",
#     }

#     payload = {
#         "header": {
#             "device_id": device["device_id"],
#             "install_id": device["install_id"],
#             "cdid": device["cdid"],
#             "openudid": device["openudid"],
#             "device_model": device["device_type"],
#             "device_brand": device["device_brand"],
#             "os": "Android",
#             "os_version": "14",
#             "app_version": "35.0.3",
#             "resolution": device["resolution"],
#         },
#         "events": [
#             {"event": "app_launch", "timestamp": rticket},
#             {"event": "session_start", "timestamp": rticket + 80},
#         ],
#         "_gen_time": rticket,
#     }

#     qs = urlencode(params)
#     payload_str = json.dumps(payload, separators=(",", ":"))
#     sig = sign(params=qs, payload=payload_str, version=8404)

#     headers = {
#         "User-Agent": f"com.zhiliaoapp.musically/2023500030 (Linux; Android 14; {device['device_type']})",
#         "Content-Type": "application/json; charset=UTF-8",
#         "x-gorgon": sig["x-gorgon"],
#         "x-khronos": sig["x-khronos"],
#         "x-argus": sig["x-argus"],
#         "x-ladon": sig["x-ladon"],
#     }

#     url = f"https://log22-normal-alisg.tiktokv.com/service/2/app_log/?{qs}"
#     r = requests.post(url, headers=headers, data=payload_str, timeout=10)

def register_device_once(device):
    ts = int(time.time())
    rticket = ts * 1000
    model, brand, resolution, dpi = device["device_type"], device["device_brand"], device["resolution"], device["dpi"]
    res_x, res_y = resolution.split("*")
    density = get_density_class(dpi)

    params = {
        "device_platform": "android",
        "os": "android",
        "ssmix": "a",
        "_rticket": rticket,
        "cdid": device["cdid"],
        "channel": "googleplay",
        "aid": "1233",
        "app_name": "musical_ly",
        "version_code": "350003",
        "version_name": "35.0.3",
        "resolution": resolution,
        "dpi": dpi,
        "device_type": model,
        "device_brand": brand,
        "language": "tr",
        "os_api": 34,
        "os_version": "14",
        "ac": "wifi",
        "is_pad": 1,
        "current_region": "TR",
        "app_type": "normal",
        "sys_region": "TR",
        "is_foldable": 1,
        "timezone_name": "Asia/Istanbul",
        "timezone_offset": 10800,
        "build_number": "35.0.3",
        "host_abi": "arm64-v8a",
        "region": "TR",
        "ts": ts,
        "iid": device["install_id"],
        "device_id": device["device_id"],
        "openudid": device["openudid"],
        "req_id": device["req_id"],
    }

    payload = {
        "header": {
            "device_model": model,
            "device_brand": brand,
            "device_manufacturer": brand,
            "os": "Android",
            "os_version": "14",
            "os_api": 34,
            "resolution": f"{res_y}x{res_x}",
            "density_dpi": dpi,
            "display_density": density,
            "display_density_v2": density,
            "resolution_v2": f"{res_y}x{res_x}",
            "openudid": device["openudid"],
            "cdid": device["cdid"],
            "install_id": device["install_id"],
            "device_id": device["device_id"],
            "google_aid": device["gaid"],
            "package": "com.zhiliaoapp.musically",
            "app_version": "35.0.3",
            "version_code": 350003,
            "update_version_code": 2023500030,
            "app_name": "musical_ly",
            "sdk_version": "2.5.0",
            "sdk_version_code": 2050090,
            "sdk_target_version": 30,
        },
        "magic_tag": "ss_app_log",
        "_gen_time": rticket,
    }

    qs = urlencode(params)
    payload_str = json.dumps(payload, separators=(",", ":"))
    sig = sign(params=qs, payload=payload_str, version=8404)

    headers = {
        "User-Agent": f"com.zhiliaoapp.musically/2023500030 (Linux; Android 14; {model})",
        "Content-Type": "application/json; charset=UTF-8",
        "x-gorgon": sig["x-gorgon"],
        "x-khronos": sig["x-khronos"],
        "x-argus": sig["x-argus"],
        "x-ladon": sig["x-ladon"],
    }

    url = f"https://log22-normal-alisg.tiktokv.com/service/2/device_register/?{qs}"
    r = requests.post(url, headers=headers, data=payload_str, timeout=10)


    try:
        resp = r.json()
        if resp.get("new_user") == 0:
            print("\n✅ Cihaz bilgileri (2. kayıt sonrası new_user=0):")
            print(json.dumps(device, indent=2))
    except:
        print("Cevap parse edilemedi.")

def register_device_full():
    model, brand, resolution, dpi = random.choice(DEVICE_POOL)
    res_x, res_y = resolution.split("*")
    density = get_density_class(dpi)

    device = {
        "device_id": _digits(),
        "install_id": _digits(),
        "openudid": _hex(),
        "cdid": str(uuid.uuid4()),
        "req_id": str(uuid.uuid4()),
        "gaid": str(uuid.uuid4()),
        "device_type": model,
        "device_brand": brand,
        "resolution": resolution,
        "dpi": dpi,
    }


    register_device_once(device)
    time.sleep(1.5)
    # send_app_log(device)
    # time.sleep(1.5)
    return device

    

