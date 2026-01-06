import time
import random
import hashlib
from curl_cffi import requests
import json
from urllib.parse import urlparse

# 导入签名库
try:
    from headers import make_headers
except ImportError:
    import sys
    import os
    sys.path.append(os.getcwd())
    from headers import make_headers

from mssdk.get_seed import seed_test
from mssdk.get_token import token_test

def get_md5(data):
    return hashlib.md5(data).hexdigest().upper()

def send_stats(item_id="7590841097820081428", pre_item_id="7559166241307561234"):
    # 基础配置
    cookies_json = {"cmpl_token":"AgQQAPMGF-ROXbhTsNJvsx0s83zPO-0Hv4qNYKPlxg","d_ticket":"656c4072fc2999ddde5428707066be9c008ae","install_id":"7556235571674433287","msToken":"qulRRMfDd3wEikEg0pyDoz-ro5Eh81Ax4NGEJIw-h3iecyC9HS3JZ0kIKFrOa3vn7_K7Gg0mtPBJIyFMvlwUoWafNntOqJDb45EoAu4yktNF1IT-k80_FWZicg==","multi_sids":"","odin_tt":"9d8881c999d8f5502f9d69c41bd9d555d7a5a6859dd6070d07957218a99a1a0287b3ca1e6bde5c4622a54ed14309e31fcf365c96435ca8c01660dded271fae3475bb1bf3b24eb181b6630e702748ddb8","passport_csrf_token":"","passport_csrf_token_default":"b512c2c9507b8b0b2a0365d8108b1ba7","sessionid":"39cc893e8b12b9dab6e2544c48255ec1","sessionid_ss":"","sid_guard":"39cc893e8b12b9dab6e2544c48255ec1%7C1763530738%7C15552000%7CMon%2C+18-May-2026+05%3A38%3A58+GMT","sid_tt":"39cc893e8b12b9dab6e2544c48255ec1","store-country-code":"mz","store-country-code-src":"uid","store-idc":"maliva","tt-target-idc":"useast1a","tt-target-idc-sign":"hNVrAVRFhYnuBF2cpqSzJHGyCvluOMnu6KU8Ptp3rgn7fazIE4doYZVxaOqTwXk1LfFINRGXH5biRl7VYzbUnIedVOHpVoG80C637Ne4AA2jL7q2HC6E5ROQwod8JIsPQWEIsNaQB0MHnF3OiQvtUFCvb8-Iu5pXYF8mAW8LS0UnzugV8M0KZqj7Cg_ole-Wc9YE7uZxkojGjYicTquo6a8WViuhWugU-yTyJ2t5McMsmM59RJMHVQmaLKCaJpB9QZFX0pboyYq8OhhBFFUaVa_43vawxeeD8hmcJwLOfiykJXF55yNoQgXPnpMRTh5NbBHzLfCswzFUuwtsMmI7MHdFKV1tP20haGubXn56DNfEcfydmyNVX7eMMlmBRxU3Aue5Pw5_r8ZzrDRPnpwr6WaSb6J12zxoljxJuVX0QFS08vrELWIwwj2RgOGY4DL3fvHjk1JmILUtRdQs29J8hqcDMJSeeQ_6Y1t5bLHW_s2HoHKbrgDzBn_GlR5uY","ttreq":"1$888c177d92713fae54619488b48ac61feb0c0f40","uid_tt":"0e617c2dcee82ff03b190e8ab88bc972c17d4ac0ed39a76d8f9f8cbc9a4f4821","uid_tt_ss":"","uid":"7524774412894192661","X-Tt-Token":"0393f3b28dfb4d259e0444b3e6f1c2e6c3029e152a34a6588936c64cffe63184566e5bd6a4b84b9f6bc139bce21bf0242276b9f01aa7616c5fc53cb2c4f38a754b9f7fb9c37a375dab73c4e00f540cbd7d5916052dfb9ec46bb9507b4f3bfe93b323d--0a4e0a207c27781baf65dc83efed105b8a5ab3e6dfe2e2183451fb1cf27092da9eb9a78912202ff1a58873314f0822a1ae78079703a44f83eebdebc923d4236283b65a847de11801220674696b746f6b-3.0.1","device_id":"7524772952601806344","ts_sign_ree":"","User-Agent":"com.zhiliaoapp.musically.go/null Dalvik/2.1.0 (Linux; U; Android 6.0; vivo Y67 Build/MRA58K)","device_brand":"vivo","device_type":"vivo Y67"}

    device_id = cookies_json.get("device_id", "7570448310440986113")
    iid = cookies_json.get("install_id", "7584536984531224337")
    model = cookies_json.get("device_type", "MI 8")
    print("device_id:",device_id)
    print("iid:",iid)
    print("model:",model)
    # 构造 Cookie 字符串
    cookie_keys = [
        "sessionid", "sid_tt", "sid_guard", "uid_tt", "uid", 
        "odin_tt", "msToken", "ttreq", "store-idc", "store-country-code", 
        "install_id", "device_id", "cmpl_token", "d_ticket", "multi_sids",
        "passport_csrf_token", "passport_csrf_token_default", "sessionid_ss",
        "store-country-code-src", "tt-target-idc", "tt-target-idc-sign",
        "ts_sign_ree"
    ]
    cookie_list = []
    for k in cookie_keys:
        if k in cookies_json and cookies_json[k]:
            cookie_list.append(f"{k}={cookies_json[k]}")
    cookie_str = "; ".join(cookie_list)

    # 当前时间
    ts = int(time.time())
    rticket = int(time.time() * 1000)

    # 1. 构造动态 URL 参数
    # 注意：某些接口校验 ts 参数是否与 Header 中的 Khronos 一致
    base_url = "https://api-core-boot.tiktokv.com/aweme/v1/aweme/stats/"
    
    version_name = "42.4.3"
    version_code = "420403"
    update_version_code = "2024204030"
    os_api = "23" # 根据 vivo Y67 (Android 6.0) 调整
    os_version = "6.0"
    
    qs_params = [
        "device_platform=android",
        "os=android",
        "ssmix=a",
        f"_rticket={rticket}",
        "channel=googleplay",
        "aid=1233",
        "app_name=musical_ly",
        f"version_code={version_code}",
        f"version_name={version_name}",
        f"manifest_version_code={update_version_code}",
        f"update_version_code={update_version_code}",
        f"ab_version={version_name}",
        "resolution=1080*2029",
        "dpi=440",
        f"device_type={model.replace(' ', '%20')}",
        f"device_brand={cookies_json.get('device_brand', 'Xiaomi')}",
        "language=zh-Hant",
        f"os_api={os_api}",
        f"os_version={os_version}",
        "ac=wifi",
        "is_pad=0",
        f"current_region={cookies_json.get('store-country-code', 'KR').upper()}",
        "app_type=normal",
        "sys_region=TW",
        "last_install_time=1767424023",
        "timezone_name=Asia%2FYerevan",
        f"residence={cookies_json.get('store-country-code', 'KR').upper()}",
        "app_language=zh-Hant",
        "timezone_offset=14400",
        "host_abi=arm64-v8a",
        "locale=zh-Hant-TW",
        "ac2=wifi5g",
        "uoo=1",
        f"op_region={cookies_json.get('store-country-code', 'KR').upper()}",
        f"build_number={version_name}",
        "region=TW",
        f"ts={ts}",
        f"iid={iid}",
        f"device_id={device_id}"
    ]
    qs = "&".join(qs_params)
    full_url = f"{base_url}?{qs}"

    # 2. 构造 Body 内容
    # 同步修改 action_time 等动态字段
    body_raw = f'pre_item_playtime=393980&user_algo_refresh_status=false&first_install_time=1762632671&enter_from=others_homepage&item_id={item_id}&is_ad=0&follow_status=0&pre_item_watch_time=1767429212007&sync_origin=false&follower_status=0&action_time={ts}&tab_type=3&pre_hot_sentence=&play_delta=1&request_id=&aweme_type=150&order=&pre_item_id={pre_item_id}'
    body_bytes = body_raw.encode('utf-8')
    body_hex = body_bytes.hex()

    # 3. 动态从 mssdk 获取 seed 和 token
    mssdk_cookie_data = cookies_json.copy()
    mssdk_cookie_data["ua"] = cookies_json.get("User-Agent", "")
    
    # 获取 seed 和 token
    # get_get_seed 返回 [seed, seed_type]
    seed_res = seed_test.get_get_seed(mssdk_cookie_data)
    seed = seed_res[0] if seed_res else ""
    seed_type = seed_res[1] if seed_res else 0
    
    # get_get_token 返回 token 字符串
    sec_device_token = token_test.get_get_token(mssdk_cookie_data)
    
    # 为 stats 类型的请求，SignCount 通常建议随请求递增，这里模拟一个较大的随机数
    sign_count = random.randint(100, 1000)
    print("seed===>",seed)
    print("seed_type===>",seed_type)
    x_ss_stub, x_khronos, x_argus, x_ladon, x_gorgon = make_headers.make_headers(
        device_id, ts, sign_count, 1, 1, ts - 60,
        sec_device_token, model, seed, seed_type, '', '', '',
        qs, body_hex,
        appVersion=version_name,
        sdkVersionStr="v05.02.02-ov-android",
        sdkVersion=0x05020220
    )

    # 4. 组装 Headers
    headers = {
        "host": "api-core-boot.tiktokv.com",
        "connection": "keep-alive",
        "x-ss-stub": x_ss_stub,
        "x-tt-pba-enable": "1",
        "x-tt-dm-status": "login=1;ct=1;rt=6",
        "x-ss-req-ticket": str(rticket),
        "sdk-version": "2",
        "passport-sdk-version": "-1",
        "x-vc-bdturing-sdk-version": "2.3.13.i18n",
        "tt-device-guard-iteration-version": "1",
        "oec-vc-sdk-version": "3.0.12.i18n",
        "x-tt-request-tag": "n=0;nr=011;bg=0",
        "x-tt-store-region": cookies_json.get("store-country-code", "tw"),
        "x-tt-store-region-src": "local",
        "user-agent": cookies_json.get("User-Agent", f"com.zhiliaoapp.musically/{update_version_code} (Linux; U; Android {os_version}; zh_TW; {model}; Build/QKQ1.190828.002;tt-ok/3.12.13.20)"),
        "x-ladon": x_ladon,
        "x-khronos": str(x_khronos),
        "x-argus": x_argus,
        "x-gorgon": x_gorgon,
        "content-type": "application/x-www-form-urlencoded; charset=UTF-8",
        "accept-encoding": "gzip, deflate",
        "cookie": cookie_str
    }
    
    if "X-Tt-Token" in cookies_json:
        headers["x-tt-token"] = cookies_json["X-Tt-Token"]

    # 5. 发送请求
    response = requests.post(
        full_url, 
        headers=headers, 
        data=body_bytes, 
        timeout=120, 
        verify=False
    )

    print(f"URL: {full_url}")
    print(f"X-Gorgon: {x_gorgon}")
    print(f"X-Argus: {x_argus}")
    print(f"X-Ladon: {x_ladon}")
    print(f"X-SS-Stub: {x_ss_stub}")
    print(f"Status Code: {response.status_code}")
    print("Response Body:")
    try:
        print(json.dumps(response.json(), indent=2, ensure_ascii=False))
    except:
        print(response.text)

if __name__ == "__main__":
    # 示例：传入活的 item_id 和 pre_item_id
    item = "7590372142106037525"
    pre_item = "7590372142106037525"
    send_stats(item_id=item, pre_item_id=pre_item)
