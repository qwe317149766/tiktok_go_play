import base64
# import datetime
import hashlib
import random
# import secrets
import time
# import urllib
# # import urllib.parse
from urllib.parse import quote, urlparse, urlencode, unquote
import uuid
# import requests
import json
from curl_cffi import requests, AsyncSession
# import requests
import urllib3
import asyncio

# from device_register.dgmain2.mwzzzh_spider import sync_parsing_logic
from device_register.dgmain2.sync_parse_logic import parse_logic
from headers.device_ticket_data import generate_delta_keypair
from mssdk.get_seed.seed_test import get_get_seed
from mssdk.get_token.token_test import get_get_token


# from mssdk.ms_dyn.report.test import DynReport
# from mssdk.ms_dyn.task.test import task

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
# from example.login.pwd_login.gorgon import make_gorgon
from headers import make_headers
from devices import getANewDevice
def url_params_to_json(url):
    result = {}
    if '?' in url:
        query_string = url.split('?', 1)[1]
    else:
        query_string=url
    pairs = query_string.split('&')
    for pair in pairs:
        if '=' in pair:
            key, value = pair.split('=', 1)
        else:
            key = pair
            value = ''
        key = unquote(key)
        value = value
        if key in result:
            if isinstance(result[key], list):
                result[key].append(value)
            else:
                result[key] = [result[key], value]
        else:
            result[key] = value
    return result
def build_query_string(params):
    return urlencode(params,safe='*').replace('%25', '%').replace('=&', '&').replace('+', '%20')


def get_md5(input_string: str) -> str:
    """
    计算给定字符串的 MD5 哈希值。
    """
    # 1. 创建 MD5 哈希对象
    hash_obj = hashlib.md5()

    # 2. 更新数据。注意：必须将 str 编码为 bytes (例如 utf-8)
    hash_obj.update(input_string.encode('utf-8'))

    # 3. 获取十六进制摘要
    hex_digest = hash_obj.hexdigest()

    return hex_digest


def get_sha256(input_string: str) -> str:
    """
    计算给定字符串的 SHA-256 哈希值。
    """
    # 1. 创建 SHA-256 哈希对象
    hash_obj = hashlib.sha256()

    # 2. 更新数据 (同样需要编码为 bytes)
    hash_obj.update(input_string.encode('utf-8'))

    # 3. 获取十六进制摘要
    hex_digest = hash_obj.hexdigest()

    return hex_digest

async def make_did_iid(device,proxy,session:AsyncSession,thread_pool,task_id):
    proxies = {
       'http': proxy,
       'https': proxy,
    }
    # --- 小工具：生成紧凑 JSON，按真机习惯转义斜杠 ---
    def to_compact_json(d: dict, escape_slash: bool = True) -> str:
        s = json.dumps(d, ensure_ascii=False, separators=(',', ':'))
        if escape_slash:
            s = s.replace('/', r'\/')
        return s

    # --- 时间与设备字段 ---
    req_id = str(uuid.uuid4())
    now = time.time()
    stime = int(now)
    utime = int(now * 1000)

    cdid = device['cdid']
    device_type = device['device_type']
    device_brand = device['device_brand']
    os_api = device['os_api']
    os_version = device['os_version']
    last_install = device['apk_last_update_time'] // 1000
    openudid = device['openudid']
    resolution = device['resolution']           # 形如 "2209x1080"
    dpi = device['dpi']
    rom = device['rom']
    resolution_v2 = device['resolution_v2']
    rom_version = device['rom_version']
    clientudid = device['clientudid']
    release_build = device['release_build']
    google_aid = device['google_aid']
    ram_size = device['ram_size']
    screen_height_dp = device['screen_height_dp']
    apk_last_update_time = device['apk_last_update_time']
    last_install_time = device['apk_last_update_time']//1000
    screen_width_dp = device['screen_width_dp']
    web_ua = device['web_ua']
    apk_first_install_time = device['apk_first_install_time']
    ua = device['ua']
    update_version_code = device['update_version_code']
    version_name = device['version_name']
    version_code = device['version_code']
    sdk_version_code = device['sdk_version_code']
    sdk_target_version = device['sdk_target_version']
    sdk_version = device['sdk_version']
    device_manufacturer = device['device_manufacturer']
    query_string1 = f"rticket={utime}&ab_version={version_name}&ac=wifi&ac2=wifi&aid=1233&app_language=en&app_name=musical_ly&app_type=normal&build_number={version_name}&carrier_region=US&carrier_region_v2=310&cdid={cdid}&channel=googleplay&device_brand={device_brand}&device_platform=android&device_type={device_type}&dpi={dpi}&host_abi=arm64-v8a&is_pad=0&language=en&last_install_time={last_install_time}&locale=en&manifest_version_code={update_version_code}&mcc_mnc=310004&op_region=US&openudid={openudid}&os=android&os_api={os_api}&os_version={os_version}&redirect_from_idc=maliva&region=US&req_id={req_id}&resolution={resolution}&ssmix=a&sys_region=US&timezone_name=America/New_York&timezone_offset=-18000&ts={stime}&uoo=0&update_version_code={update_version_code}&version_code={version_code}&version_name={version_name}"
    query_string = "&".join(
        f"{k}={quote(v, safe='*').replace('%25', '%')}"
        for param in query_string1.split("&")
        if "=" in param
        for k, v in [param.split("=", 1)]
    ).replace(' ', '%20')
    # 用这个 EXACT 的 query 作为 URL 的一部分（不要再用另一个变量混用）
    url = f"https://log-boot.tiktokv.com/service/2/device_register/?{query_string}"
    # params = build_query_string(url_params_to_json(url))
    # query_string = params
    # -------- 2) 构造 EXACT 同真机格式的 JSON body（参与签名 & 实际发送都用这一个）--------
    body_dict = {
        "header": {
            "os": "Android",
            "os_version": str(os_version),
            "os_api": int(os_api),
            "device_model": device_type,
            "device_brand": device_brand,      # 抓包里多见小写
            "device_manufacturer": device_manufacturer,
            "cpu_abi": "arm64-v8a",
            "density_dpi": int(dpi),
            "display_density": "mdpi",
            "resolution": resolution,                   # 这里依你的真机抓包：body 是 "2209x1080"
            "display_density_v2": "xxhdpi",
            "resolution_v2": resolution_v2,
            "access": "wifi",
            "rom": rom,
            "rom_version": rom_version,
            "language": "en",
            "timezone": -4,
            "region": "US",                             # 抓包 body 里有 "region":"US"
            "tz_name": "America/New_York",
            "tz_offset": -14400,
            "clientudid": clientudid,
            "openudid": openudid,
            "channel": "googleplay",
            "not_request_sender": 1,
            "aid": 1233,
            "release_build": release_build,
            "ab_version": version_name,
            "google_aid": google_aid,
            "gaid_limited": 0,
            "custom": {
                "ram_size": str(ram_size),
                "dark_mode_setting_value": 1,
                "is_foldable": 0,
                "screen_height_dp": int(screen_height_dp),
                "apk_last_update_time": int(apk_last_update_time),
                "filter_warn": 0,
                "priority_region": "US",
                "user_period": 0,
                "is_kids_mode": 0,
                "web_ua": web_ua,
                "screen_width_dp": int(screen_width_dp),
                "user_mode": 1,                        # 抓包可见 -1；若你已固定为 1，也要与抓包一致
            },
            "package": "com.zhiliaoapp.musically",
            "app_version": version_name,
            "app_version_minor": "",
            "version_code": int(version_code),
            "update_version_code": int(update_version_code),
            "manifest_version_code": int(update_version_code),
            "app_name": "musical_ly",
            "tweaked_channel": "googleplay",
            "display_name": "TikTok",
            # "sig_hash": "194326e82c84a639a52e5c023116f12a",  # 若抓包里有且参与签名，必须放开并一致
            "cdid": cdid,
            "device_platform": "android",
            # "git_hash": "5151884",
            "sdk_version_code": sdk_version_code,
            "sdk_target_version": sdk_target_version,
            "req_id": req_id,
            "sdk_version": sdk_version,
            "guest_mode": 0,
            "sdk_flavor": "i18nInner",
            "apk_first_install_time": int(apk_first_install_time),
            "is_system_app": 0
        },
        "magic_tag": "ss_app_log",
        "_gen_time": utime
    }

    body_json = to_compact_json(body_dict,
                                escape_slash=False
                                )     # 与真机字节一致（\/）
    # print(body_json)
    body_bytes = body_json.encode('utf-8')
    body_hex   = body_bytes.hex()                                 # 视你的 make_headers 需要

    # -------- 3) 生成签名：务必用 “相同的 query_string + 相同的 body_hex” --------
    # 你自己的签名器接口：请用与“真实发送”一致的两个参数
    x_ss_stub, x_khronos, x_argus, x_ladon, x_gorgon = make_headers.make_headers(
        "",                         # device_id 可空时就空（与抓包一致）
        stime,                      # Khronos = 秒级时间戳
        random.randint(20, 40),
        random.randint(100, 500),
        random.randint(100, 500),
        stime - random.randint(50, 100),
        "",                         # token
        device_type,
        '', '', '', '', '',
        query_string,               # ⚠️一定是“最终发送的”query（和 URL 完全一致）
        body_hex                    # ⚠️一定是“最终发送的”body 的 hex
    )
    # -------- 4) 发送：URL + body 与签名输入完全一致；修正 header 值 --------
    headers = {
        "Host": "log-boot.tiktokv.com",
        "x-ss-stub": x_ss_stub,
        "x-tt-app-init-region": "carrierregion=;mccmnc=;sysregion=US;appregion=US",
        "x-tt-request-tag": "t=0;n=1",
        "x-tt-dm-status": "login=0;ct=0;rt=1",
        "x-ss-req-ticket": str(utime),
        "sdk-version": "2",
        "passport-sdk-version": "-1",
        "x-vc-bdturing-sdk-version": "2.3.13.i18n",
        "user-agent": ua,
        "x-ladon": x_ladon,
        "x-khronos": str(x_khronos),    # ✅ 这里必须是秒级时间戳，不要写成 x_gorgon
        "x-argus": x_argus,
        "x-gorgon": x_gorgon,
        "content-type": "application/json; charset=utf-8",
        "accept-encoding": "gzip",
    }

    # 发送时：URL里就是上面那个 query_string；body 就是 body_json
    # if proxy!="":
    #     resp = requests.post(url, headers=headers, data=body_json, timeout=15,
    #                          proxies=proxies,
    #                          verify=False,
    #                          impersonate="chrome131_android" # Match User-Agent profile
    #                          )
    # else:
    #     resp = requests.post(url, headers=headers, data=body_json, timeout=15,
    #                          # proxies=proxies,
    #                          # verify=False,
    #                          impersonate="chrome131_android" # Match User-Agent profile
    #                          )
    # print(1111)
    resp = await session.post(url, headers=headers, data=body_json, timeout=15,
                         proxies=proxies,
                         verify=False,
                         # impersonate="chrome131_android"
                         )
    loop = asyncio.get_running_loop()

    # 注意：如果你的解析函数需要多个参数，直接跟在后面写
    # 这里的 resp.text 是传给 sync_parsing_logic 的第一个参数
    # task_id 是第二个参数
    result = await loop.run_in_executor(
        thread_pool,
        parse_logic,
        0,  # type = 0
        resp,
        device
    )
    return result

async def alert_check(device,proxy,session:AsyncSession,thread_pool,task_id):
    proxies = {
        'http': proxy,
        'https': proxy,
    }
    now = time.time()
    utime = int(now * 1000)  # 毫秒
    stime = int(now)  # 秒
    # install_time_s = stime - random.randint(300, 1000)  # 你原来的习惯
    req_id = str(uuid.uuid4())
    cdid = device["cdid"]
    open_uid = device["openudid"]
    # phoneInfo = device["phoneInfo"]
    device_id = device["device_id"]
    # device_id = "7566195049035286030"
    install_id = device["install_id"]
    # install_id = "7566195504888792845"

    apk_last_update_time = device["apk_last_update_time"]
    last_install_time = apk_last_update_time // 1000
    resolution = device["resolution"]
    dpi = device["dpi"]
    device_type = device["device_type"]
    device_brand = device["device_brand"]
    os_api = device["os_api"]
    os_version = device["os_version"]
    ua = device["ua"]
    iid = device["install_id"]
    update_version_code = device['update_version_code']
    version_name = device['version_name']
    version_code = device['version_code']
    sdk_version_code = device['sdk_version_code']
    sdk_target_version = device['sdk_target_version']
    sdk_version = device['sdk_version']
    openudid = device['openudid']
    # 1. Use the full URL with all parameters to prevent auto-encoding issues.
    # Note: The 'tt_info' parameter already seems to be base64 encoded, which is fine.
    tt_info = f"device_platform=android&os=android&ssmix=a&_rticket={utime}&cdid={cdid}&channel=googleplay&aid=1233&app_name=musical_ly&version_code={version_code}&version_name={version_name}&manifest_version_code={update_version_code}&update_version_code={update_version_code}&ab_version={version_name}&resolution={resolution}&dpi={dpi}&device_type={device_type}&device_brand={device_brand}&language=en&os_api={os_api}&os_version={os_version}&ac=wifi&is_pad=0&current_region=US&app_type=normal&sys_region=US&last_install_time={last_install_time}&timezone_name=America/New_York&residence=US&app_language=en&timezone_offset=-18000&host_abi=arm64-v8a&locale=en&ac2=wifi&uoo=0&op_region=US&build_number={version_name}&region=US&ts={stime}&iid={iid}&device_id={device_id}&openudid={open_uid}&req_id={req_id}&google_aid={device['google_aid']}&gaid_limited=0&timezone=-5.0&custom_bt=1761217864104"
    tt_info = base64.b64encode(bytes(tt_info, "utf-8")).decode("utf-8")
    # print(tt_info)
    query_string1 = f"rticket={utime}&ab_version={version_name}&ac=wifi&ac2=wifi&aid=1233&app_language=en&app_name=musical_ly&app_type=normal&build_number={version_name}&carrier_region=US&carrier_region_v2=310&cdid={cdid}&channel=googleplay&device_brand={device_brand}&device_platform=android&device_type={device_type}&dpi={dpi}&host_abi=arm64-v8a&is_pad=0&language=en&last_install_time={last_install_time}&locale=en&manifest_version_code={update_version_code}&mcc_mnc=310004&op_region=US&openudid={openudid}&os=android&os_api={os_api}&os_version={os_version}&redirect_from_idc=maliva&region=US&req_id={req_id}&resolution={resolution}&ssmix=a&sys_region=US&timezone_name=America/New_York&timezone_offset=-18000&ts={stime}&uoo=0&update_version_code={update_version_code}&version_code={version_code}&version_name={version_name}"
    query_string = "&".join(
        f"{k}={quote(v, safe='*').replace('%25', '%')}"
        for param in query_string1.split("&")
        if "=" in param
        for k, v in [param.split("=", 1)]
    ).replace(' ', '%20')
    url = f"https://log-boot.tiktokv.com/service/2/app_alert_check/?{query_string}"


    post_data = ""
    # ---------- 生成签名（传入真实 query 与 body 原始 bytes） ----------
    x_ss_stub, x_khronos, x_argus, x_ladon, x_gorgon = make_headers.make_headers(
        device_id,  # 依你的实现
        stime,
        random.randint(20, 40),
        2,
        4,
        stime - random.randint(1, 10),
        "",
        device_type,  # 用真实 model
        "", "", "", "", "",
        query_string,
        post_data  # 注意：传 bytes，不是 hex
    )
    # 2. Use a list of tuples for headers to strictly preserve the exact order.
    # Note: The ":authority:", ":method:", ":path:", ":scheme:" are HTTP/2 pseudo-headers
    # and are handled differently by HTTP clients. We map them to standard HTTP/1.1 headers.
    headers = [
        ('accept-encoding', 'gzip'),
        ('x-tt-app-init-region', 'carrierregion=;mccmnc=;sysregion=US;appregion=US'),
        ('x-tt-dm-status', 'login=0;ct=0;rt=1'),
        ('x-ss-req-ticket', f'{utime}'),
        ('sdk-version', '2'),
        ('passport-sdk-version', '-1'),
        ('x-vc-bdturing-sdk-version', '2.3.13.i18n'),
        ('user-agent',
         device['ua']),
        ('x-ladon', f'{x_ladon}'),
        ('x-khronos', f'{x_khronos}'),
        ('x-argus',
         f'{x_argus}'),
        ('x-gorgon', f'{x_gorgon}'),
        # Map HTTP/2 pseudo-headers to standard headers
        ('Host', 'log-boot.tiktokv.com'),  # from :authority:
        # No direct mapping for :method:, :path:, :scheme: as they are part of the request line/URL
    ]

    response = await session.get(
        url,
        headers=dict(headers),
        proxies=proxies,
        verify=False,
        # impersonate="chrome131_android"
    )
    loop = asyncio.get_running_loop()

    # 注意：如果你的解析函数需要多个参数，直接跟在后面写
    # 这里的 resp.text 是传给 sync_parsing_logic 的第一个参数
    # task_id 是第二个参数
    result = await loop.run_in_executor(
        thread_pool,
        parse_logic,
        1,  # type = 1
        response,
        device
    )
    return result

async def make_ds_sign(device,proxy,session:AsyncSession,thread_pool,task_id):
    proxies = {
        'http': proxy,
        'https': proxy,
    }
    now = time.time()
    utime = int(now * 1000)  # 毫秒
    stime = int(now)  # 秒
    # install_time_s = stime - random.randint(300, 1000)  # 你原来的习惯
    req_id = str(uuid.uuid4())
    cdid = device["cdid"]
    open_uid = device["openudid"]
    # phoneInfo = device["phoneInfo"]
    device_id = device["device_id"]
    # device_id = "7566195049035286030"
    install_id = device["install_id"]
    # install_id = "7566195504888792845"

    apk_last_update_time = device["apk_last_update_time"]
    last_install_time = apk_last_update_time // 1000
    resolution = device["resolution"]
    dpi = device["dpi"]
    device_type = device["device_type"]
    device_brand = device["device_brand"]
    os_api = device["os_api"]
    os_version = device["os_version"]
    ua = device["ua"]
    iid = device["install_id"]
    update_version_code = device['update_version_code']
    version_name = device['version_name']
    version_code = device['version_code']
    sdk_version_code = device['sdk_version_code']
    sdk_target_version = device['sdk_target_version']
    sdk_version = device['sdk_version']
    openudid = device['openudid']
    google_aid = device['google_aid']
    device_manufacturer = device['device_manufacturer']
    query_string1 =f"from=normal&from_error&device_platform=android&os=android&ssmix=a&_rticket={utime}&cdid={cdid}&channel=googleplay&aid=1233&app_name=musical_ly&version_code={version_code}&version_name={version_name}&manifest_version_code={update_version_code}&update_version_code={update_version_code}&ab_version={version_name}&resolution={resolution}&dpi={dpi}&device_type={device_type}&device_brand={device_brand}&language=en&os_api={os_api}&os_version={os_version}&ac=wifi&is_pad=0&app_type=normal&sys_region=US&last_install_time={last_install_time}&mcc_mnc=310004&timezone_name=America%2FNew_York&carrier_region_v2=310&app_language=en&carrier_region=US&ac2=wifi&uoo=0&op_region=US&timezone_offset=-18000&build_number={version_name}&host_abi=arm64-v8a&locale=en&region=US&ts={stime}&iid={install_id}&device_id={device_id}&openudid={openudid}"
    query_string = "&".join(
        f"{k}={quote(v, safe='*').replace('%25', '%')}"
        for param in query_string1.split("&")
        if "=" in param
        for k, v in [param.split("=", 1)]
    ).replace(' ', '%20')

    url = f"https://aggr16-normal.tiktokv.us/service/2/dsign/?{query_string}"
    data = f'{{"device_id":"{device_id}","install_id":"{install_id}","aid":1233,"app_version":"{version_name}","model":"{device_type}","os":"Android","openudid":"{openudid}","google_aid":"{google_aid}","properties_version":"android-1.0","device_properties":{{"device_model":"{get_sha256((device_type))}","device_manufacturer":"{get_sha256(device_manufacturer)}","disk_size":"ea489ffb302814b62320c02536989a3962de820f5a481eb5bac1086697d9aa3c","memory_size":"291cf975c42a1e788fdc454e3c7330d641db5f9f7ba06e37f7f388b3448bc374","resolution":"{get_sha256(resolution)}","re_time":"0af7de3d5239bb5542f0653e57c7c8b9","indss18":"8725063fe010181646c25d1f993e1589","indc15":"7874453cef13dddd56fcb3c7e8e99c28","indn5":"a9ca935c4885bbc1da2be687f153354c","indmc14":"e678d34e71a6943f1cab0bfa3c7a226b","inda0":"d0eac42291b9a88173d9914972a65d8b","indal2":"d7baecabd462bc9f960eaab4c81a55c5","indm10":"446ae4837d88b3b3988d57b9747e11cd","indsp3":"9861cb1513b66e9aaeb66ef048bfdd18","indsd8":"a15ec37e1115dea871970a39ec0769c4","bl":"a3d41c6f3e8c1892d2cc97469805b1f0","cmf":"5494690cb9b316eb618265ea11dc5146","bc":"1e2b66f4392214037884408109a383df","stz":"e6f9d2069f89b53a8e6f2c65929d2e50","sl":"2389ca43e5adab9de01d2dda7633ac39"}}}}'
    post_data = data.encode('utf-8').hex()
    # ---------- 生成签名（传入真实 query 与 body 原始 bytes） ----------
    x_ss_stub, x_khronos, x_argus, x_ladon, x_gorgon = make_headers.make_headers(
        device_id,  # 依你的实现
        stime,
        random.randint(20, 40),
        2,
        4,
        stime - random.randint(1, 10),
        "",
        device_type,  # 用真实 model
        "", "", "", "", "",
        query_string,
        post_data  # 注意：传 bytes，不是 hex
    )
    DeltaKeyPair = generate_delta_keypair()
    tt_ticket_guard_public_key = DeltaKeyPair.tt_public_key_b64
    priv_key = DeltaKeyPair.priv_hex
    cookie_lines = [
        "store-idc=useast5",
        "store-country-code=us",
        "store-country-code-src=did",
        # "store-country-sign=MEIEDI4vJkem3-cZBJP_1QQgpDj6ZMbTFnAUFxxPYBtDR97cIQTZod_SM9sAiA-Xf2QEECkqEtolqJ59ahDftGt1cuc",
        f"install_id={install_id}",
        # "ttreq=1$9205bebb95b0c9de217ca4017890aabf70010e33"
    ]
    cookie_string = "; ".join(cookie_lines)
    headers = [
        # ('content-length', '1330'),
        ('cookie', cookie_string),
        ('x-tt-request-tag', 't=0;n=1'),
        ('tt-ticket-guard-public-key',
         tt_ticket_guard_public_key),
        ('sdk-version', '2'),
        ('x-tt-dm-status', 'login=0;ct=0;rt=1'),
        ('x-ss-req-ticket', f'{utime}'),
        ('tt-device-guard-iteration-version', '1'),
        ('passport-sdk-version', '-1'),
        ('x-vc-bdturing-sdk-version', '2.3.17.i18n'),
        ('content-type', 'application/json; charset=utf-8'),
        ('x-ss-stub', f'{x_ss_stub}'),
        ('rpc-persist-pyxis-policy-state-law-is-ca', '1'),
        ('rpc-persist-pyxis-policy-v-tnc', '1'),
        ('x-tt-ttnet-origin-host', 'log16-normal-useast8.tiktokv.us'),
        ('x-ss-dp', '1233'),
        # ('x-tt-trace-id', '00-64ab57e1010c5fc028f5b51b644d04d1-64ab57e1010c5fc0-01'),
        ('user-agent',
         ua),
        ('accept-encoding', 'gzip, deflate'),
        # ('x-argus',
        #  x_argus),
        # ('x-gorgon', f'{x_gorgon}'),
        # ('x-khronos', f'{x_khronos}'),
        # ('x-ladon', f'{x_ladon}'),
        ('Host', 'aggr16-normal.tiktokv.us'),  # 从 :authority: 映射而来
    ]

    response = await session.post(
        url,
        headers=dict(headers),
        data=data,
        proxies=proxies,
        verify=False,
        # impersonate="chrome131_android"
    )

    loop = asyncio.get_running_loop()

    # 注意：如果你的解析函数需要多个参数，直接跟在后面写
    # 这里的 resp.text 是传给 sync_parsing_logic 的第一个参数
    # task_id 是第二个参数
    result = await loop.run_in_executor(
        thread_pool,
        parse_logic,
        2,  # type = 2
        response,
        device,
        tt_ticket_guard_public_key,  # args[0]
        priv_key  # args[1]
    )
    return result



async def run_registration_flow(session: AsyncSession, proxy: str | None,thread_pool,task_id) -> dict | None:
    """
    执行完整的设备注册流程。
    成功则返回 device_data 字典，失败则返回 None。
    """
    when = None
    try:
        device = getANewDevice()
        # print(111)
        # 步骤 1: make_did_iid
        device1, device_id = await make_did_iid(device, proxy, session,thread_pool,task_id)
        when = "make_did_iid"
        # print(222)
        if device_id != 0:
            # 步骤 2: alert_check
            when = "alert_check"
            res1 = await alert_check(device1, proxy, session,thread_pool,task_id)
            # print(333)
            if res1 and res1 == "success":
                when = "make_ds_sign"
                # 步骤 3: make_ds_sign
                print(device1)
                dsign_result = await make_ds_sign(device1, proxy, session,thread_pool,task_id)
                # print(444)
                if dsign_result:
                    device_guard_data0, tt_ticket_guard_public_key, priv_key = dsign_result
                    device1["device_guard_data0"] = device_guard_data0
                    device1["tt_ticket_guard_public_key"] = tt_ticket_guard_public_key
                    device1["priv_key"] = priv_key

                    ## 修正：取消注释这些步骤
                    ## 假设你已经用真实的、异步的 mssdk 替换了 headers.py 中的模拟函数
                    # seed, seed_type = await get_get_seed(device, proxy, session)
                    # token = await get_get_token(device, proxy, session)
                    # device1["seed"] = seed
                    # device1["seed_type"] = seed_type
                    # device1["token"] = token

                    # 流程成功完成
                    return device1

    except Exception as e:
        print(f"注册流程中发生错误: {e}")
        # 可以在这里添加更详细的日志

    # 任何步骤失败都会导致返回 None
    return when

