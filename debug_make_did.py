import base64
import hashlib
import random
import time
import uuid
import json
import asyncio
import gzip
from io import BytesIO
from urllib.parse import quote, unquote
from curl_cffi import requests
import urllib3

# 禁用 InsecureRequestWarning
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

# 尝试导入签名库，如果失败则需要检查路径
try:
    from headers import make_headers
except ImportError:
    print("⚠️ 无法导入 headers.make_headers，请确保 PYTHONPATH 包含当前目录或 headers 文件夹存在")
    # 如果路径不对，尝试手动修复 path 或提供 mock
    import sys
    import os
    sys.path.append(os.getcwd())
    from headers import make_headers

def to_compact_json(d: dict, escape_slash: bool = False) -> str:
    s = json.dumps(d, ensure_ascii=False, separators=(',', ':'))
    if escape_slash:
        s = s.replace('/', r'\/')
    return s

async def debug_make_did_iid():
    # --- 1. 设备配置 (与 register_debug.go 保持一致: TW + log-boot.tiktokv.com) ---
    domain = "log-boot.tiktokv.com"
    region = "TW"
    language = "zh-Hant"
    tz_name = "Asia/Yerevan"
    tz_offset = 14400
    
    # 使用随机值进行测试
    device = {
        "cdid": str(uuid.uuid4()),
        "openudid": "".join([random.choice("0123456789abcdef") for _ in range(16)]),
        "clientudid": str(uuid.uuid4()),
        "req_id": str(uuid.uuid4()),
        "device_type": "MI 8",
        "device_brand": "Xiaomi",
        "device_manufacturer": "Xiaomi",
        "os_api": "29",
        "os_version": "10",
        "resolution": "2029x1080",
        "resolution_v2": "2248x1080",
        "dpi": 440,
        "rom": "MIUI-V12.5.2.0.QEACNXM",
        "rom_version": "miui_V125_V12.5.2.0.QEACNXM",
        "release_build": "4ca920e_20250626",
        "ram_size": "6GB",
        "screen_height_dp": 817,
        "screen_width_dp": 393,
        "web_ua": "Dalvik/2.1.0 (Linux; U; Android 10; MI 8 Build/QKQ1.190828.002)",
        "ua": "com.zhiliaoapp.musically/2024204030 (Linux; U; Android 10; zh_TW; MI 8; Build/QKQ1.190828.002;tt-ok/3.12.13.20)",
        "apk_last_update_time": int(time.time() * 1000),
        "apk_first_install_time": int(time.time() * 1000),
        "version_name": "42.4.3",
        "version_code": "420403",
        "update_version_code": "2024204030",
        "sdk_version_code": 2050990,
        "sdk_target_version": 30,
        "sdk_version": "2.5.9",
        "google_aid": str(uuid.uuid4()),
        "domain": domain,
        "region": region,
        "language": language,
        "tz_name": tz_name,
        "tz_offset": tz_offset
    }

    # proxy = "http://127.0.0.1:7890" 
    # proxies = {"http": proxy, "https": proxy}

    # --- 2. 构造 Query String ---
    req_id = device['req_id']
    stime = 1767424033
    utime = 1767424033997
    last_install_time = 1767424023
    
    # 构造原始参数串 (顺序参考抓包)
    rticket = utime
    res_query = "1080*2029"

    query_params = {
        "req_id": req_id,
        "device_platform": "android",
        "os": "android",
        "ssmix": "a",
        "_rticket": rticket,
        "cdid": device['cdid'],
        "channel": "googleplay",
        "aid": "1233",
        "app_name": "musical_ly",
        "version_code": device['version_code'],
        "version_name": device['version_name'],
        "manifest_version_code": device['update_version_code'],
        "update_version_code": device['update_version_code'],
        "ab_version": device['version_name'],
        "resolution": res_query,
        "dpi": device['dpi'],
        "device_type": device['device_type'],
        "device_brand": device['device_brand'],
        "language": "zh-Hant",
        "os_api": device['os_api'],
        "os_version": device['os_version'],
        "ac": "wifi",
        "is_pad": 0,
        "app_type": "normal",
        "sys_region": region,
        "last_install_time": last_install_time,
        "timezone_name": tz_name,
        "app_language": "zh-Hant",
        "timezone_offset": tz_offset,
        "host_abi": "arm64-v8a",
        "locale": "zh-Hant-TW",
        "ac2": "unknown",
        "uoo": 1,
        "op_region": region,
        "build_number": device['version_name'],
        "region": region,
        "ts": stime,
        "openudid": device['openudid'],
        "okhttp_version": "4.2.228.18-tiktok",
        "use_store_region_cookie": 1
    }

    # 手动拼接确保顺序 (非常重要，影响签名)
    qs = "&".join([f"{k}={quote(str(v), safe='*').replace('%25', '%')}" for k, v in query_params.items()]).replace(' ', '%20')
    url = f"https://{domain}/service/2/device_register/?{qs}"

    # --- 3. 构造 Body ---
    body_dict = {
        "header": {
            "os": "Android",
            "os_version": str(device['os_version']),
            "os_api": int(device['os_api']),
            "device_model": device['device_type'],
            "device_brand": "Xiaomi",
            "device_manufacturer": "Xiaomi",
            "cpu_abi": "arm64-v8a",
            "density_dpi": int(device['dpi']),
            "display_density": "mdpi",
            "resolution": device['resolution'], 
            "display_density_v2": "xxhdpi",
            "resolution_v2": device['resolution_v2'],
            "access": "wifi",
            "rom": device['rom'],
            "rom_version": device['rom_version'],
            "language": "zh",
            "timezone": 4,
            "region": region,
            "tz_name": tz_name,
            "tz_offset": tz_offset,
            "clientudid": device['clientudid'],
            "openudid": device['openudid'],
            "channel": "googleplay",
            "not_request_sender": 1,
            "aid": 1233,
            "release_build": device['release_build'],
            "ab_version": device['version_name'],
            "google_aid": device['google_aid'],
            "gaid_limited": 0,
            "custom": {
                "ram_size": device['ram_size'],
                "dark_mode_setting_value": 1,
                "is_foldable": 0,
                "screen_height_dp": device['screen_height_dp'],
                "apk_last_update_time": int(device['apk_last_update_time']),
                "filter_warn": 0,
                "priority_region": region,
                "user_period": 0,
                "is_kids_mode": 0,
                "web_ua": device['web_ua'],
                "screen_width_dp": device['screen_width_dp'],
                "user_mode": -1,
            },
            "package": "com.zhiliaoapp.musically",
            "app_version": device['version_name'],
            "app_version_minor": "",
            "version_code": int(device['version_code']),
            "update_version_code": int(device['update_version_code']),
            "manifest_version_code": int(device['update_version_code']),
            "app_name": "musical_ly",
            "tweaked_channel": "googleplay",
            "display_name": "TikTok",
            "sig_hash": "194326e82c84a639a52e5c023116f12a",
            "cdid": device['cdid'],
            "device_platform": "android",
            "sdk_version_code": device['sdk_version_code'],
            "sdk_target_version": device['sdk_target_version'],
            "req_id": req_id,
            "sdk_version": device['sdk_version'],
            "guest_mode": 0,
            "sdk_flavor": "i18nInner",
            "apk_first_install_time": int(device['apk_first_install_time']),
            "is_system_app": 0
        },
        "magic_tag": "ss_app_log",
        "_gen_time": rticket
    }

    body_json = to_compact_json(body_dict, escape_slash=False)
    body_bytes = body_json.encode('utf-8')
    body_hex = body_bytes.hex()
    # print(f"[DEBUG] Body Hex: {body_hex}")

    # --- 4. 生成签名 ---
    # 使用抓包时的 count
    x_ss_stub, x_khronos, x_argus, x_ladon, x_gorgon = make_headers.make_headers(
        "", stime, 1, 0, 0, stime,
        "", device['device_type'], '', '', '', '', '',
        qs, body_hex
    )

    # --- 5. 发送请求 ---
    headers = {
        "Host": domain,
        "x-ss-stub": x_ss_stub,
        "x-tt-app-init-region": f"carrierregion=;mccmnc=;sysregion={region};appregion={region}",
        "x-tt-request-tag": "t=0;n=1",
        "x-tt-dm-status": "login=0;ct=0;rt=7",
        "x-ss-req-ticket": str(utime),
        "sdk-version": "2",
        "passport-sdk-version": "-1",
        "x-vc-bdturing-sdk-version": "2.3.13.i18n",
        "user-agent": device['ua'],
        "x-ladon": x_ladon,
        "x-khronos": str(x_khronos),
        "x-argus": x_argus,
        "x-gorgon": x_gorgon,
        "content-type": "application/json; charset=utf-8",
        "accept-encoding": "gzip",
    }

    print(f"\n[INFO] 发送请求到: {url}")
    print(f"X-Gorgon: {x_gorgon}")
    
    # 使用 curl_cffi 发送
    with requests.Session() as s:
        # 如果需要代理
        # s.proxies = proxies
        resp = s.post(url, headers=headers, data=body_bytes, timeout=15, verify=False)
        
        print(f"\nHTTP 状态码: {resp.status_code}")
        # 将响应体写入文件
        with open("response_body.bin", "wb") as f:
            f.write(resp.content)
        
        print("--- 响应头 ---")
        for k, v in resp.headers.items():
            print(f"{k}: {v}")
        print("--------------")

        try:
            resp_json = resp.json()
            print("[OK] 响应解析成功:")
            print(json.dumps(resp_json, indent=2, ensure_ascii=False))
            
            # 尝试从 body 提取
            did = resp_json.get("device_id_str") or resp_json.get("device_id")
            iid = resp_json.get("install_id_str") or resp_json.get("install_id")
            
            # 如果 body 里没有，尝试从 cookie 提取 (有时注册成功但在 cookie 里返回)
            if not did or did == "0" or did == 0:
                did = s.cookies.get("device_id")
            if not iid or iid == "0" or iid == 0:
                iid = s.cookies.get("install_id")
                
            print(f"\n[DONE] 提取结果 -> Device ID: {did}, Install ID: {iid}")
            
            if not did or not iid:
                print("[ERR] 未能获取有效的 Device ID 或 Install ID，停止后续测试")
                return
            
            # 继续测试 DSign
            server_data, priv_key = await debug_make_ds_sign(device, did, iid, s)
            
            # 测试播放接口 (Stats)
            if server_data:
                await debug_make_stats(device, did, iid, server_data, s, priv_key)
        except Exception as e:
            print(f"[ERR] 发生错误: {e}")
            import traceback
            traceback.print_exc()

async def debug_make_ds_sign(device, did, iid, session):
    stime = int(time.time())
    utime = int(time.time() * 1000)
    
    # 使用正确的模块导入 generate_delta_keypair
    priv_key = "" 
    pub_key_b64 = ""
    try:
        from headers.device_ticket_data import generate_delta_keypair
        kp = generate_delta_keypair()
        priv_key = kp.priv_hex
        pub_key_b64 = kp.tt_public_key_b64
        print(f"[DEBUG] Generated DSign PubKey: {pub_key_b64[:30]}...")
    except Exception as e:
        print(f"[ERR] 导入或生成 Keypair 失败: {e}")
        # 如果失败，DSign 肯定会报错，我们先给个随机 Base64 看看能不能绕过初步检查（虽然基本不可能成功）
        pub_key_b64 = base64.b64encode(secrets.token_bytes(65)).decode()

    def get_sha256(s):
        return hashlib.sha256(s.encode()).hexdigest()

    body_dict = {
        "device_id": did,
        "install_id": iid,
        "aid": 1233,
        "app_version": device['version_name'],
        "model": device['device_type'],
        "os": "Android",
        "openudid": device['openudid'],
        "google_aid": device['google_aid'],
        "properties_version": "android-1.0",
        "device_properties": {
            "device_model": get_sha256(device['device_type']),
            "device_manufacturer": get_sha256(device['device_brand']),
            "disk_size": "ea489ffb302814b62320c02536989a3962de820f5a481eb5bac1086697d9aa3c",
            "memory_size": "291cf975c42a1e788fdc454e3c7330d641db5f9f7ba06e37f7f388b3448bc374",
            "resolution": get_sha256(device['resolution']),
            "re_time": "0af7de3d5239bb5542f0653e57c7c8b9",
            "indss18": "8725063fe010181646c25d1f993e1589",
            "indc15": "7874453cef13dddd56fcb3c7e8e99c28",
            "indn5": "a9ca935c4885bbc1da2be687f153354c",
            "indmc14": "e678d34e71a6943f1cab0bfa3c7a226b",
            "inda0": "d0eac42291b9a88173d9914972a65d8b",
            "indal2": "d7baecabd462bc9f960eaab4c81a55c5",
            "indm10": "446ae4837d88b3b3988d57b9747e11cd",
            "indsp3": "9861cb1513b66e9aaeb66ef048bfdd18",
            "indsd8": "a15ec37e1115dea871970a39ec0769c4",
            "bl": "a3d41c6f3e8c1892d2cc97469805b1f0",
            "cmf": "5494690cb9b316eb618265ea11dc5146",
            "bc": "1e2b66f4392214037884408109a383df",
            "stz": "e6f9d2069f89b53a8e6f2c65929d2e50",
            "sl": "2389ca43e5adab9de01d2dda7633ac39"
        }
    }

    body_json = to_compact_json(body_dict)
    body_hex = body_json.encode().hex()

    query_params = {
        "from": "normal",
        "from_error": "", # 补齐 from_error
        "device_platform": "android",
        "os": "android",
        "ssmix": "a",
        "_rticket": utime,
        "cdid": device['cdid'],
        "channel": "googleplay",
        "aid": "1233",
        "app_name": "musical_ly",
        "version_code": device['version_code'],
        "version_name": device['version_name'],
        "manifest_version_code": device['update_version_code'],
        "update_version_code": device['update_version_code'],
        "ab_version": device['version_name'],
        "resolution": "1080*2029",
        "dpi": device['dpi'],
        "device_type": device['device_type'],
        "device_brand": device['device_brand'],
        "language": device['language'],
        "os_api": device['os_api'],
        "os_version": device['os_version'],
        "ac": "wifi",
        "is_pad": 0,
        "app_type": "normal",
        "sys_region": device['region'],
        "last_install_time": (utime//1000) - 86400,
        "timezone_name": device['tz_name'],
        "app_language": device['language'],
        "timezone_offset": device['tz_offset'],
        "host_abi": "arm64-v8a",
        "locale": "zh-Hant-TW",
        "ac2": "unknown",
        "uoo": 1,
        "op_region": device['region'],
        "build_number": device['version_name'],
        "region": device['region'],
        "ts": stime,
        "iid": iid,
        "device_id": did,
        "openudid": device['openudid']
    }

    qs = "&".join([f"{k}={quote(str(v), safe='*').replace('%25', '%')}" if v != "" else k for k, v in query_params.items()]).replace(' ', '%20')
    
    x_ss_stub, x_khronos, x_argus, x_ladon, x_gorgon = make_headers.make_headers(
        did, stime, random.randint(20, 40), 2, 4, stime - 5,
        "", device['device_type'], '', '', '', '', '',
        qs, body_hex
    )

    # 同步 register_logic.py 的 Header 逻辑
    cookie_string = f"install_id={iid}; store-idc=alisg; store-country-code=tw; store-country-code-src=did"
    headers = {
        "cookie": cookie_string,
        "x-tt-request-tag": "t=0;n=1",
        "tt-ticket-guard-public-key": pub_key_b64,
        "sdk-version": "2",
        "x-tt-dm-status": "login=0;ct=0;rt=1",
        "x-ss-req-ticket": str(utime),
        "tt-device-guard-iteration-version": "1",
        "passport-sdk-version": "-1",
        "x-vc-bdturing-sdk-version": "2.3.17.i18n",
        "content-type": "application/json; charset=utf-8",
        "x-ss-stub": x_ss_stub,
        "rpc-persist-pyxis-policy-state-law-is-ca": "1",
        "rpc-persist-pyxis-policy-v-tnc": "1",
        "x-tt-ttnet-origin-host": "log22-normal-alisg.tiktokv.com",
        "x-ss-dp": "1233",
        "user-agent": device['ua'],
        "accept-encoding": "gzip, deflate",
    }

    url = f"https://{device['domain']}/service/2/dsign/?{qs}"
    print(f"\n[INFO] 发送 DSign: {url}")
    
    resp = session.post(url, headers=headers, data=body_json.encode(), timeout=15, verify=False)
    print(f"DSign HTTP 状态码: {resp.status_code}")
    print("--- DSign 响应头 ---")
    for k, v in resp.headers.items():
        print(f"{k}: {v}")
    print("-------------------")
    
    # print(f"DSign 响应内容: {resp.text}")
    server_data = resp.headers.get("tt-device-guard-server-data")
    print(f"[DONE] DSign Server Data: {server_data[:50] if server_data else 'None'}")
    return server_data, priv_key

async def debug_make_stats(device, did, iid, server_data, session, priv_key_hex):
    print(f"\n[INFO] 开始测试 Stats (Play) 接口...")
    
    # 1. 解析 server_data (base64 -> json)
    try:
        decoded_guard = json.loads(base64.b64decode(server_data).decode('utf-8'))
        device_guard_data0 = {
            "device_token": decoded_guard.get("device_token"),
            "dtoken_sign": decoded_guard.get("dtoken_sign")
        }
    except Exception as e:
        print(f"[ERR] 解析 server_data 失败: {e}")
        return

    # 2. 调用 build_guard
    from headers.device_ticket_data import build_guard
    guard_headers = build_guard(
        device_guard_data0=device_guard_data0,
        path="/aweme/v1/aweme/stats/",
        priv_hex=priv_key_hex
    )

    # 3. 时间戳
    stime = int(time.time())
    utime = stime * 1000
    last_install_time = 1767424023 # 固定一个参考值，或者使用 device['apk_first_install_time']//1000

    # 4. 构造 Query String (参考 send_stats.py)
    query_params = {
        "device_platform": "android",
        "os": "android",
        "ssmix": "a",
        "_rticket": utime,
        "channel": "googleplay",
        "aid": "1233",
        "app_name": "musical_ly",
        "version_code": "420403",
        "version_name": "42.4.3",
        "manifest_version_code": "2024204030",
        "update_version_code": "2024204030",
        "ab_version": "42.4.3",
        "resolution": "1080*2029",
        "dpi": 440,
        "device_type": device['device_type'],
        "device_brand": device['device_brand'],
        "language": device['language'],
        "os_api": device['os_api'],
        "os_version": device['os_version'],
        "ac": "wifi",
        "is_pad": 0,
        "current_region": "TW",
        "app_type": "normal",
        "sys_region": "TW",
        "last_install_time": last_install_time,
        "timezone_name": device['tz_name'],
        "residence": "TW",
        "app_language": device['language'],
        "timezone_offset": device['tz_offset'],
        "host_abi": "arm64-v8a",
        "locale": "zh-Hant-TW",
        "ac2": "wifi5g",
        "uoo": 1,
        "op_region": "TW",
        "build_number": "42.4.3",
        "region": "TW",
        "ts": stime,
        "iid": iid,
        "device_id": did
    }
    
    qs = "&".join([f"{k}={quote(str(v), safe='*').replace('%25', '%')}" for k, v in query_params.items()]).replace(' ', '%20')

    # 5. 构造 POST Body (参考 send_stats.py)
    aweme_id = "7590841097820081428"
    dt = f'pre_item_playtime=393980&user_algo_refresh_status=false&first_install_time=1762632671&enter_from=others_homepage&item_id={aweme_id}&is_ad=0&follow_status=0&pre_item_watch_time={utime-393980}&sync_origin=false&follower_status=0&action_time={stime}&tab_type=3&pre_hot_sentence=&play_delta=1&request_id=&aweme_type=150&order=&pre_item_id=7559166241307561234'
    
    body_hex = dt.encode().hex()

    # 6. 生成签名 (make_headers)
    x_ss_stub, x_khronos, x_argus, x_ladon, x_gorgon = make_headers.make_headers(
        did, stime, random.randint(100, 1000), 1, 1, stime - 60,
        "", device['device_type'], "", 0, "", "", "",
        qs, body_hex,
        appVersion="42.4.3", sdkVersionStr="v05.02.02-ov-android", sdkVersion=0x05020220
    )

    # 7. 组装 Headers (参考 send_stats.py)
    headers = {
        "host": "api-core-boot.tiktokv.com",
        "connection": "keep-alive",
        "x-ss-stub": x_ss_stub,
        "x-tt-pba-enable": "1",
        "x-tt-dm-status": "login=0;ct=1;rt=6",
        "x-ss-req-ticket": str(utime),
        "sdk-version": "2",
        "passport-sdk-version": "-1",
        "x-vc-bdturing-sdk-version": "2.3.13.i18n",
        "rpc-persist-pns-region-1": "TW|1835841|1843561",
        "rpc-persist-pns-region-2": "TW|1835841|1843561",
        "rpc-persist-pns-region-3": "TW|1835841|1843561",
        "tt-device-guard-iteration-version": "1",
        "tt-ticket-guard-public-key": guard_headers["tt-ticket-guard-public-key"],
        "tt-device-guard-client-data": guard_headers["tt-device-guard-client-data"],
        "oec-vc-sdk-version": "3.0.12.i18n",
        "x-tt-request-tag": "n=0;nr=011;bg=0",
        "x-tt-store-region": "tw",
        "x-tt-store-region-src": "local",
        "user-agent": device['ua'],
        "x-ladon": x_ladon,
        "x-khronos": str(x_khronos),
        "x-argus": x_argus,
        "x-gorgon": x_gorgon,
        "content-type": "application/x-www-form-urlencoded; charset=UTF-8",
        "accept-encoding": "gzip, deflate"
    }

    # 8. Gzip 压缩 Body
    out = BytesIO()
    with gzip.GzipFile(fileobj=out, mode='w') as f:
        f.write(dt.encode())
    body_gzipped = out.getvalue()

    # 9. 发送请求
    url = f"https://api-core-boot.tiktokv.com/aweme/v1/aweme/stats/?{qs}"
    print(f"\n[INFO] 发送 Stats (Play): {url}")
    
    resp = session.post(url, headers=headers, data=body_gzipped, timeout=15, verify=False)
    print(f"Stats HTTP 状态码: {resp.status_code}")
    print("--- Stats 响应头 ---")
    for k, v in resp.headers.items():
        print(f"{k}: {v}")
    
    if not resp.text.strip():
        print(f"Stats 响应体为空，Raw Hex: {resp.content.hex()}")
    else:
        print(f"Stats 响应内容: {resp.text}")

if __name__ == "__main__":
    asyncio.run(debug_make_did_iid())
