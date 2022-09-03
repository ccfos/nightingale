#!/usr/bin/env python
# -*- coding: UTF-8 -*-
import sys
import json

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
    def send_feishu(cls, payload):
        # already done in go code
        pass

    @classmethod
    def send_mm(cls, payload):
        # already done in go code
        pass

    @classmethod
    def send_sms(cls, payload):
        users = payload.get('event').get("notify_users_obj")
        phones = {}
        for u in users:
            if u.get("phone"):
                phones[u.get("phone")] = 1
        if phones:
            print("send_sms not implemented, phones: {}".format(phones.keys()))

    @classmethod
    def send_voice(cls, payload):
        users = payload.get('event').get("notify_users_obj")
        phones = {}
        for u in users:
            if u.get("phone"):
                phones[u.get("phone")] = 1
        if phones:
            print("send_voice not implemented, phones: {}".format(phones.keys()))

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