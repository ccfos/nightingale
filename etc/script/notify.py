#!/usr/bin/python
# -*- coding: UTF-8 -*-
#
# n9e-server把告警事件通过stdin的方式传入notify.py，notify.py从事件中解析出接收人信息、拼出通知内容，发送通知
# 脚本的灵活性高，要接入短信、电话、jira、飞书等，都非常容易，只要有接口，notify.py去调用即可
#
import sys
import json
import os
import smtplib
import time
import requests
from email.mime.text import MIMEText
from email.header import Header
from bottle import template

reload(sys)                      # reload 才能调用 setdefaultencoding 方法
sys.setdefaultencoding('utf-8')  # 设置 'utf-8'

################################
##    邮件告警，修改下面的配置    ##
################################
mail_host = "smtp.163.com"
mail_port = 994
mail_user = "ulricqin"
mail_pass = "password"
mail_from = "ulricqin@163.com"

# 本地告警event json存储目录
LOCAL_EVENT_FILE_DIR = ".alerts"
NOTIFY_CHANNELS_SPLIT_STR = " "

NOTIFY_CHANNEL_DICT = {
  "email":"email",
  "sms":"sms",
  "voice":"voice",
  "dingtalk":"dingtalk",
  "wecom":"wecom"
}

# stdin 告警json实例
TEST_ALERT_JSON = {
    "event": {
        "alert_duration": 10,
        "notify_channels": "dingtalk",
        "res_classpaths": "all",
        "id": 4,
        "notify_group_objs": None,
        "rule_note": "",
        "history_points": [
            {
                "metric": "go_goroutines",
                "points": [
                    {
                        "t": 1625213114,
                        "v": 33.0
                    }
                ],
                "tags": {
                    "instance": "localhost:9090",
                    "job": "prometheus"
                }
            }
        ],
        "priority": 1,
        "last_sent": True,
        "tag_map": {
            "instance": "localhost:9090",
            "job": "prometheus"
        },
        "hash_id": "ecb258d2ca03454ee390a352913c461b",
        "status": 0,
        "tags": "instance=localhost:9090 job=prometheus",
        "trigger_time": 1625213114,
        "res_ident": "ident1",
        "rule_name": "alert_test",
        "is_prome_pull": 1,
        "notify_users": "1",
        "notify_groups": "",
        "runbook_url": "",
        "values": "[vector={__name__=\"go_goroutines\", instance=\"localhost:9090\", job=\"prometheus\"}]: [value=33.000000]",
        "readable_expression": "go_goroutines>0",
        "notify_user_objs": None,
        "is_recovery": 1,
        "rule_id": 1
    },
    "rule": {
        "alert_duration": 10,
        "notify_channels": "dingtalk",
        "enable_stime": "00:00",
        "id": 1,
        "note": "",
        "create_by": "root",
        "append_tags": "",
        "priority": 1,
        "update_by": "root",
        "type": 1,
        "status": 0,
        "recovery_notify": 0,
        "enable_days_of_week": "1 2 3 4 5 6 7",
        "callbacks": "localhost:10000",
        "notify_users": "1",
        "notify_groups": "",
        "runbook_url": "",
        "name": "a",
        "update_at": 1625211576,
        "create_at": 1625211576,
        "enable_etime": "23:59",
        "group_id": 1,
        "expression": {
            "evaluation_interval": 4,
            "promql": "go_goroutines>0"
        }
    },
    "users": [
        {
            "username": "root",
            "status": 0,
            "contacts": {
                "dingtalk_robot_token": "xxxxxx"
            },
            "create_by": "system",
            "update_at": 1625211432,
            "create_at": 1624871926,
            "email": "",
            "phone": "",
            "role": "Admin",
            "update_by": "root",
            "portrait": "",
            "nickname": "\u8d85\u7ba1",
            "id": 1
        }
    ]
}


def main():
    payload = json.load(sys.stdin)
    trigger_time = payload['event']['trigger_time']
    event_id = payload['event']['id']
    rule_id = payload['rule']['id']
    notify_channels = payload['event'].get('notify_channels').strip().split(NOTIFY_CHANNELS_SPLIT_STR)
    if len(notify_channels) == 0:
        msg = "notify_channels_empty"
        print(msg)
        return
    # 持久化到本地json文件
    persist(payload, rule_id, event_id, trigger_time)
    # 生成告警内容
    alert_content = sms_content_gen(values_gen(payload))
    for ch in notify_channels:
        send_func_name = "send_{}".format(NOTIFY_CHANNEL_DICT.get(ch.strip()))
        has_func = hasattr(Send, send_func_name)

        if not has_func:
            msg = "[send_func_name_err][func_not_found_in_Send_class:{}]".format(send_func_name)
            print(msg)
            continue
        send_func = getattr(Send, send_func_name)
        send_func(alert_content, payload)

def values_gen(payload):
    event_obj = payload.get("event")
    values = {
        "IsAlert": event_obj.get("is_recovery") == 0,
        "IsMachineDep": event_obj.get("res_classpaths") != "",
        "Status": status_gen(event_obj.get("priority"),event_obj.get("is_recovery")),
        "Sname": event_obj.get("rule_name"),
        "Ident": event_obj.get("res_ident"),
        "Classpath": event_obj.get("res_classpaths"),
        "Metric": metric_gen(event_obj.get("history_points")),
        "Tags": event_obj.get("tags"),
        "Value": event_obj.get("values"),
        "ReadableExpression": event_obj.get("readable_expression"),
        "TriggerTime": time.strftime("%Y-%m-%d %H:%M:%S",time.localtime(event_obj.get("trigger_time"))),
        "Elink": "http://n9e.didiyun.com/strategy/edit/{}".format(event_obj.get("rule_id")),
        "Slink": "http://n9e.didiyun.com/event/{}".format(event_obj.get("id"))
    }

    return values

def email_content_gen(values):
    return template('etc/script/tpl/mail.tpl', values)

def sms_content_gen(values):
    return template('etc/script/tpl/sms.tpl', values)

def status_gen(priority,is_recovery):
    is_recovery_str_m = {1: "恢复", 0: "告警"}
    status = "P{} {}".format(priority, is_recovery_str_m.get(is_recovery))
    return status

def subject_gen(priority,is_recovery,rule_name):
    is_recovery_str_m = {1: "恢复", 0: "告警"}
    subject = "P{} {} {}".format(priority, is_recovery_str_m.get(is_recovery), rule_name)
    return subject

def metric_gen(history_points):
    metrics = [] 
    for item in history_points:
        metrics.append(item.get("metric"))
    return ",".join(metrics)

def persist(payload, rule_id, event_id, trigger_time):
    if not os.path.exists(LOCAL_EVENT_FILE_DIR):
        os.makedirs(LOCAL_EVENT_FILE_DIR)

    filename = '%d_%d_%d' % (rule_id, event_id, trigger_time)
    filepath = os.path.join(LOCAL_EVENT_FILE_DIR, filename)
    with open(filepath, 'w') as f:
        f.write(json.dumps(payload, indent=4))


class Send(object):
    @classmethod
    def send_email(cls, alert_content, payload):
        users = payload.get("users")
        emails = [x.get("email") for x in users]
        if not emails:
            return

        recipients = emails
        mail_body = email_content_gen(values_gen(payload))
        message = MIMEText(mail_body, 'html', 'utf-8')
        message['From'] = mail_from
        message['To'] = ", ".join(recipients)
        message["Subject"] = subject_gen(payload.get("event").get("priority"),payload.get("event").get("is_recovery"),payload.get("event").get("rule_name"))

        smtp = smtplib.SMTP_SSL(mail_host, mail_port)
        smtp.login(mail_user, mail_pass)
        smtp.sendmail(mail_from, recipients, message.as_string())
        smtp.close()

        print("send_mail_success")

    @classmethod
    def send_wecom(cls, alert_content, payload):
        users = payload.get("users")

        for u in users:
            contacts = u.get("contacts")
            wecom_robot_token = contacts.get("wecom_robot_token", "")

            if wecom_robot_token == "":
                continue

            wecom_api_url = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key={}".format(wecom_robot_token)
            atMobiles = [u.get("phone")]
            headers = {'Content-Type': 'application/json;charset=utf-8'}
            payload = {
                "msgtype": "text",
                "text": {
                    "content": alert_content
                },
                "at": {
                    "atMobiles": atMobiles,
                    "isAtAll": False
                }
            }
            res = requests.post(wecom_api_url, json.dumps(payload), headers=headers)
            print(res.status_code)
            print(res.text)
            print("send_wecom")


    @classmethod
    def send_dingtalk(cls, alert_content, payload):
        # 钉钉发群信息需要群的webhook机器人 token，这个信息可以在user的contacts map中

        users = payload.get("users")

        for u in users:
            contacts = u.get("contacts")

            dingtalk_robot_token = contacts.get("dingtalk_robot_token", "")

            if dingtalk_robot_token == "":
                print("dingtalk_robot_token_not_found")
                continue

            dingtalk_api_url = "https://oapi.dingtalk.com/robot/send?access_token={}".format(dingtalk_robot_token)
            atMobiles = [u.get("phone")]
            headers = {'Content-Type': 'application/json;charset=utf-8'}
            payload = {
                "msgtype": "text",
                "text": {
                    "content": alert_content
                },
                "at": {
                    "atMobiles": atMobiles,
                    "isAtAll": False
                }
            }
            res = requests.post(dingtalk_api_url, json.dumps(payload), headers=headers)
            print(res.status_code)
            print(res.text)

            print("send_dingtalk")


def mail_test():
    print("mail_test_todo")

    recipients = ["ulricqin@qq.com", "ulric@163.com"]

    payload =  json.loads(json.dumps(TEST_ALERT_JSON))
    mail_body = email_content_gen(values_gen(payload))
    message = MIMEText(mail_body, 'html', 'utf-8')
    message['From'] = mail_from
    message['To'] = ", ".join(recipients)
    message["Subject"] = subject_gen(payload.get("event").get("priority"),payload.get("event").get("is_recovery"),payload.get("event").get("rule_name"))

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

