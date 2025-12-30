import base64


def parse_logic(type:int,resp,device,*args): # type表示当前请求种类,0为make_did_iid，1为alert_check,2为make_ds_sign
    '''

    :param type:
    :param resp:
    :param device:
    :param args:  tt_ticket_guard_public_key priv_key
    :return:
    '''
    if type == 0:  # 解析make_did_iid
        j = resp.json()
        device_id = j.get('device_id')
        install_id = j.get('install_id')
        device["device_id"] = str(device_id) if device_id is not None else ""
        device["install_id"] = str(install_id) if install_id is not None else ""
        print(f"device_id: {device_id}, install_id: {install_id}")
        return [device, device_id]
    elif type ==1: # 解析alert_check
        if resp.text == '{"message":"success"}':
            # print("did、iid 激活成功")
            return "success"
        else:
            return None
    elif type ==2:
        res = resp.json()
        print("make_ds_signnnnn",res)
        tt_ticket_guard_public_key = args[0]
        priv_key = args[1]
        device_guard_server_data = res["tt-device-guard-server-data"]
        res1 = base64.b64decode(device_guard_server_data).decode()
        return [res1, tt_ticket_guard_public_key, priv_key]
