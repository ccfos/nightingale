#!/usr/bin/python
# -*- coding: UTF-8 -*-
import sys
import json
import os
import smtplib
from email.mime.text import MIMEText
from email.header import Header

# 希望的demo实现效果：
# 1. 从stdin拿到告警信息之后，格式化为一个有缩进的json写入一个临时文件
# 2. 文件路径和名字是.alerts/${timestamp}_${ruleid}
# 3. 调用SMTP服务器发送告警，微信、钉钉、飞书、slack、jira、短信、电话等等留给社区实现

mail_host = "smtp.163.com"
mail_port = 994
mail_user = "ulricqin"
mail_pass = "password"
mail_from = "ulricqin@163.com"

# just for test
mail_body = """
<p>邮件发送测试</p>
<p><a href="https://www.baidu.com">baidu</a></p>
"""

def main():
    payload = json.load(sys.stdin)
    persist(payload)


def persist(payload):
    if not os.path.exists(".alerts"):
        os.makedirs(".alerts")

    filename = '%d_%d' % (payload['event']['trigger_time'], payload['rule']['id'])
    filepath = os.path.join(".alerts", filename)

    f = open(filepath, 'w')
    print(json.dumps(payload, indent=4), file=f)
    f.close()


def send_mail(payload):
    print("send_mail")


def send_wecom(payload):
    print("send_wecom")


def send_dingtalk(payload):
    print("send_dingtalk")


def mail_test():
    print("mail_test_todo")

    recipients = ["ulricqin@qq.com", "ulric@163.com"]
    
    message = MIMEText(mail_body, 'html', 'utf-8')
    message['From'] = mail_from
    message['To'] = ", ".join(recipients)
    message["Subject"] = "n9e alert"

    smtp = smtplib.SMTP_SSL(mail_host, mail_port)
    smtp.login(mail_user, mail_pass)
    smtp.sendmail(mail_from, recipients, message.as_string())
    smtp.close()

    print("mail_test_done")


if __name__ == "__main__":
    if len(sys.argv) == 1:
        main()
    elif sys.argv[1] == "mail":
        mail_test()
    else:
        print("I am confused")