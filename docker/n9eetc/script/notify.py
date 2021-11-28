#!/usr/bin/python
# -*- coding: UTF-8 -*-
import sys
import json
import urllib2
import smtplib
from email.mime.text import MIMEText

notify_channel_funcs = {
  "email":"email",
  "sms":"sms",
  "voice":"voice",
  "dingtalk":"dingtalk",
  "wecom":"wecom"
}

mail_host = "smtp.163.com"
mail_port = 994
mail_user = "ulricqin"
mail_pass = "password"
mail_from = "ulricqin@163.com"

class Sender(object):
    @classmethod
    def send_email(cls, payload):
        users = payload.get('event').get("notify_users_obj")

        emails = {}
        for u in users:
            if u.get("email"):
                emails[u.get("email")] = 1

        if not emails:
            return

        recipients = emails.keys()
        mail_body = payload.get('tpls').get("mailbody.tpl", "mailbody.tpl not found")
        message = MIMEText(mail_body, 'html', 'utf-8')
        message['From'] = mail_from
        message['To'] = ", ".join(recipients)
        message["Subject"] = payload.get('tpls').get("subject.tpl", "subject.tpl not found")

        try:
            smtp = smtplib.SMTP_SSL(mail_host, mail_port)
            smtp.login(mail_user, mail_pass)
            smtp.sendmail(mail_from, recipients, message.as_string())
            smtp.close()
        except smtplib.SMTPException, error:
            print(error)

    @classmethod
    def send_wecom(cls, payload):
        users = payload.get('event').get("notify_users_obj")

        tokens = {}

        for u in users:
            contacts = u.get("contacts")
            if contacts.get("wecom_robot_token", ""):
                tokens[contacts.get("wecom_robot_token", "")] = 1

        opener = urllib2.build_opener(urllib2.HTTPHandler())
        method = "POST"

        for t in tokens:
            url = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key={}".format(t)
            body = {
                "msgtype": "markdown",
                "markdown": {
                    "content": payload.get('tpls').get("wecom.tpl", "wecom.tpl not found")
                }
            }
            request = urllib2.Request(url, data=json.dumps(body))
            request.add_header("Content-Type",'application/json;charset=utf-8')
            request.get_method = lambda: method
            try:
                connection = opener.open(request)
                print(connection.read())
            except urllib2.HTTPError, error:
                print(error)

    @classmethod
    def send_dingtalk(cls, payload):
        users = payload.get('event').get("notify_users_obj")

        tokens = {}
        phones = {}

        for u in users:
            if u.get("phone"):
                phones[u.get("phone")] = 1

            contacts = u.get("contacts")
            if contacts.get("dingtalk_robot_token", ""):
                tokens[contacts.get("dingtalk_robot_token", "")] = 1

        opener = urllib2.build_opener(urllib2.HTTPHandler())
        method = "POST"

        for t in tokens:
            url = "https://oapi.dingtalk.com/robot/send?access_token={}".format(t)
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
            request = urllib2.Request(url, data=json.dumps(body))
            request.add_header("Content-Type",'application/json;charset=utf-8')
            request.get_method = lambda: method
            try:
                connection = opener.open(request)
                print(connection.read())
            except urllib2.HTTPError, error:
                print(error)

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