#!/usr/bin/env python
# -*- coding: UTF-8 -*-
import sys
import json
import requests

class Sender(object):
    @classmethod
    def send_email(cls, payload):
        # already done in go code
        pass

    @classmethod
    def send_wecom(cls, payload):
        # already done in go code
        pass

    @classmethod
    def send_dingtalk(cls, payload):
        # already done in go code
        pass

    @classmethod
    def send_ifeishu(cls, payload):
        users = payload.get('event').get("notify_users_obj")
        tokens = {}
        phones = {}

        for u in users:
            if u.get("phone"):
                phones[u.get("phone")] = 1

            contacts = u.get("contacts")
            if contacts.get("feishu_robot_token", ""):
                tokens[contacts.get("feishu_robot_token", "")] = 1
        
        headers = {
            "Content-Type": "application/json;charset=utf-8",
            "Host": "open.feishu.cn"
        }

        for t in tokens:
            url = "https://open.feishu.cn/open-apis/bot/v2/hook/{}".format(t)
            body = {
                "msg_type": "text",
                "content": {
                    "text": payload.get('tpls').get("feishu", "feishu not found")
                },
                "at": {
                    "atMobiles": list(phones.keys()),
                    "isAtAll": False
                }
            }

            response = requests.post(url, headers=headers, data=json.dumps(body))
            print(f"notify_ifeishu: token={t} status_code={response.status_code} response_text={response.text}")

    @classmethod
    def send_mm(cls, payload):
        # already done in go code
        pass

    @classmethod
    def send_sms(cls, payload):
        pass

    @classmethod
    def send_voice(cls, payload):
        pass

def main():
    payload = json.load(sys.stdin)
    with open(".payload", 'w') as f:
        f.write(json.dumps(payload, indent=4))
    for ch in payload.get('event').get('notify_channels'):
        send_func_name = "send_{}".format(ch.strip())
        if not hasattr(Sender, send_func_name):
            print("function: {} not found", send_func_name)
            continue
        send_func = getattr(Sender, send_func_name)
        send_func(payload)

def hello():
    print("hello nightingale")

if __name__ == "__main__":
    if len(sys.argv) == 1:
        main()
    elif sys.argv[1] == "hello":
        hello()
    else:
        print("I am confused")