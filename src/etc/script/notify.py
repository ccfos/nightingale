#!/usr/bin/env python
# -*- coding: UTF-8 -*-
import sys
import json
import urllib.request

notify_channel_funcs = {
  "dingtalk":"dingtalk"
}

class Sender(object):

    @classmethod
    def send_dingtalk(cls, payload):
        users = payload.get('event').get("notify_users_obj")
        print("users: ", users)

        tokens = {}
        for u in users:
            contacts = u.get("contacts")
            if contacts.get("dingtalk_robot_token", ""):
                tokens[contacts.get("dingtalk_robot_token", "")] = 1
             
        for t in tokens:
            url = "https://oapi.dingtalk.com/robot/send?access_token={}".format(t)
            header = {
                    'Content-Type': 'application/json;charset=utf-8'
            }
            #isAtAll 是否所有人发送0 false  1 true
            body = {
                    "msgtype": "text",
                    "text": {
                        "content": payload.get('tpls').get("dingtalk.tpl", "dingtalk.tpl not found")
                    },
                    "at": {
                        "isAtAll": 0
                    }
            }
            print(body)
            data = json.dumps(body)
            data = bytes(data, 'utf8')
            request = urllib.request.Request(url,data = data,headers = header)
            response = urllib.request.urlopen(request)
            page = response.read().decode('utf-8')
            print(page)

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
    print(payload)
    with open(".payload", 'w') as f:
        f.write(json.dumps(payload, indent=4))
    for ch in payload.get('event').get('notify_channels'):
        send_func_name = "send_{}".format(notify_channel_funcs.get(ch.strip()))
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