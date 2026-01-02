import requests
import gzip
import time
from urllib.parse import urlparse
from SignerPy import sign
from deviceRegister import register_device_full

def get_dynamic_payload(item_id):
    ts = int(time.time())
    return (
        f"pre_item_playtime=915&user_algo_refresh_status=false"
        f"&first_install_time={ts - 50000}"
        f"&item_id={item_id}&is_ad=0&follow_status=0"
        f"&sync_origin=false&follower_status=0"
        f"&action_time={ts}&tab_type=22&pre_hot_sentence="
        f"&play_delta=1&request_id=&aweme_type=0&order="
    ), ts

def send_view_request(device, item_id):
    payload_str, ts = get_dynamic_payload(item_id)
    payload_bytes = payload_str.encode("utf-8")
    payload_gzip = gzip.compress(payload_bytes)

    _rticket = ts * 1000

    url = (
        f"https://api31-core-alisg.tiktokv.com/aweme/v1/aweme/stats/"
        f"?os=android&_rticket={_rticket}&is_pad=1"
        f"&last_install_time={ts - 20000}&is_foldable=1"
        f"&host_abi=arm64-v8a&ts={ts}&"
    )
    x_common_params = (
        f"ab_version=35.0.3&ac=wifi&ac2=wifi&aid=1233&app_language=tr"
        f"&app_name=musical_ly&app_type=normal&build_number=35.0.3"
        f"&cdid={device['cdid']}&channel=googleplay&current_region=TR"
        f"&device_brand={device['device_brand']}&device_id={device['device_id']}"
        f"&device_platform=android&device_type={device['device_type']}"
        f"&dpi={device['dpi']}&iid={device['install_id']}&language=tr"
        f"&locale=tr-TR&manifest_version_code=2023500030&op_region=TR"
        f"&openudid={device['openudid']}&os_api=34&os_version=14"
        f"&region=TR&residence=TR&resolution={device['resolution']}"
        f"&ssmix=a&sys_region=TR&timezone_name=Asia%2FIstanbul"
        f"&timezone_offset=10800&uoo=0&update_version_code=2023500030"
        f"&version_code=350003&version_name=35.0.3"
    )

    parsed = urlparse(url)
    url_query = parsed.query
    full_params = url_query + x_common_params

    signature = sign(params=full_params, payload=payload_str, version=8404)

    headers = {
        "User-Agent": f"com.zhiliaoapp.musically/2023500030 (Linux; Android 14; tr_TR; {device['device_type']}; Build/UP1A.231005.007)",
        "Connection": "keep-alive",
        "Accept": "*/*",
        "Accept-Encoding": "gzip, deflate, br",
        "content-type": "application/x-www-form-urlencoded; charset=UTF-8",
        "x-bd-content-encoding": "gzip",
        "x-common-params-v2": x_common_params,
        "x-gorgon": signature["x-gorgon"],
        "x-khronos": signature["x-khronos"],
        "x-ladon": signature["x-ladon"],
        "x-argus": signature["x-argus"],
    }

    try:
        response = requests.post(url, data=payload_gzip, headers=headers, timeout=10)
        if response.status_code == 200:
            json_data = response.json()
            # now = json_data.get("extra", {}).get("now")
            print(f"✅ İzlenme gönderildi. Response: {json_data}")
        else:
            print(f"⚠️ Hata [{response.status_code}] - {response.text}")
    except Exception as e:
        print(f"❌ İstek Hatası: {str(e)}")

if __name__ == "__main__":
    device = register_device_full()

    item_id = input("İzlenme Gönderilecek Video ID'si (video id): ")

    try:
        adet = int(input("Kaç İzlenme Gönderilsin: "))
    except:
        adet = 1

    for i in range(adet):
        print(f"\n📤 Gönderim {i+1}/{adet}")
        send_view_request(device, item_id)
        time.sleep(1)
