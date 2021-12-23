#!/usr/bin/env python
# -*- coding: UTF-8 -*-
import sys
import json
# import urllib2
import urllib.request

notify_channel_funcs = {
  "dingtalk":"dingtalk"
}

class Sender(object):

    @classmethod
    def send_dingtalk(cls, payload):
        print('send_dingtalk')

        users = payload.get('event').get("notify_users_obj")
        print("users: ", users)  # users:  [{'id': 1, 'username': 'root', 'nickname': '超管', 'phone': '', 'email': '', 'portrait': '', 'roles': ['Admin'], 'contacts': {}, 'create_at': 1639971175, 'create_by': 'system', 'update_at': 1639971175, 'update_by': 'system', 'admin': False}]

        tokens = {}
        phones = {}

        for u in users:
            if u.get("phone"):
                phones[u.get("phone")] = 1

            contacts = u.get("contacts")
            if contacts.get("dingtalk_robot_token", ""):
                tokens[contacts.get("dingtalk_robot_token", "")] = 1
             
        # opener = urllib2.build_opener(urllib2.HTTPHandler())
        method = "POST"

        for t in tokens:
            url = "https://oapi.dingtalk.com/robot/send?access_token={}".format(t)
            print(url)
            body = {
                "msgtype": "text",
                "text": {
                    "content": payload.get('tpls').get("dingtalk.tpl", "dingtalk.tpl not found")
                },
                "at": {
                    "atMobiles": phones.keys(),
                    "isAtAll": False
                }
            }
            # request = urllib.request(url, data=json.dumps(body))
            # request = urllib2.Request(url, data=json.dumps(body))
            # request.add_header("Content-Type",'application/json;charset=utf-8')
            # request.get_method = lambda: method
            # try:
            #     request = request.urlopen(url)
            #     response = urllib.request.urlopen(request)
            #     # connection = opener.open(request)
            #     # print(connection.read())
            #     print(response.read().decode('utf-8'))
            # # except urllib2.HTTPError, error:
            # #     print(error)
            # except error:
            #     print(error)

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