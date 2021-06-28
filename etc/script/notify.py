#!/usr/bin/python
# -*- coding: UTF-8 -*-
import sys
import json
import os
import smtplib
import time
from email.mime.text import MIMEText
from email.header import Header

# 希望的demo实现效果：
# 1. 从stdin拿到告警信息之后，格式化为一个有缩进的json写入一个临时文件
# 2. 文件路径和名字是.alerts/${timestamp}_${ruleid}
# 3. 调用SMTP服务器发送告警，微信、钉钉、飞书、slack、jira、短信、电话等等留给社区实现
import requests

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

# 本地告警event json存储目录
LOCAL_EVENT_FILE_DIR = ".alerts"
NOTIFY_CHANNELS_SPLIT_STR = " "

# stdin 告警json实例
TEST_ALERT_JSON = {
    "event": {
        "alert_duration": 30,
        "hash_id": "0c6cbe29df08bb6f9c26af518638128e",
        "history_points": [
            {
                "metric": "go_gc_duration_seconds",
                "points": [
                    {
                        "t": 1624851995,
                        "v": 0.000175
                    }
                ],
                "tags": {
                    "instance": "172.20.70.205:9100",
                    "job": "node-targets",
                    "quantile": "0.25"
                }
            }
        ],
        "id": 190,
        "is_prome_pull": 1,
        "is_recovery": 0,
        "last_sent": True,
        "notify_channels": "qq dingding ",
        "notify_group_objs": None,
        "notify_groups": "",
        "notify_user_objs": None,
        "notify_users": "1",
        "priority": 1,
        "readable_expression": " go_gc_duration_seconds>0",
        "res_classpaths": "all",
        "res_ident": "",
        "rule_id": 10,
        "rule_name": "pull_promql",
        "rule_note": "note",
        "runbook_url": "qq.com",
        "status": 0,
        "tag_map": {
            "a": "b",
            "c": "d",
            "instance": "172.20.70.205:9100",
            "job": "node-targets",
            "quantile": "0.25"
        },
        "tags": "a=b c=d instance=172.20.70.205:9100 job=node-targets quantile=0.25",
        "trigger_time": 1624851995,
        "values": "[vector={__name__=\"go_gc_duration_seconds\", instance=\"172.20.70.205:9100\", job=\"node-targets\", quantile=\"0.25\"}]: [value=0.000175]"
    },
    "rule": {
        "alert_duration": 30,
        "append_tags": "a=b c=d",
        "callbacks": "localhost:8881",
        "create_at": 1624851512,
        "create_by": "root",
        "enable_days_of_week": "1 2 3 4 5 6 7",
        "enable_etime": "23:59",
        "enable_stime": "00:00",
        "expression": {
            "evaluation_interval": 3,
            "promql": " go_gc_duration_seconds>0",
            "resolve_timeout": 40
        },
        "group_id": 1,
        "id": 10,
        "name": "pull_promql",
        "note": "note",
        "notify_channels": "qq dingding ",
        "notify_groups": "",
        "notify_users": "1",
        "priority": 1,
        "recovery_notify": 0,
        "runbook_url": "qq.com",
        "status": 0,
        "type": 1,
        "update_at": 1624851512,
        "update_by": "root"
    },
    "users": [
        {
            "contacts": None,
            "create_at": 1624258550,
            "create_by": "system",
            "email": "",
            "id": 1,
            "nickname": "\u8d85\u7ba1",
            "phone": "",
            "portrait": "",
            "role": "Admin",
            "status": 0,
            "update_at": 1624258550,
            "update_by": "system",
            "username": "root"
        }
    ]
}


def main():
    payload = json.load(sys.stdin)
    trigger_time = payload['event']['trigger_time']
    rule_id = payload['rule']['id']
    notify_channels = payload['event'].get('notify_channels').strip().split(NOTIFY_CHANNELS_SPLIT_STR)
    if len(notify_channels) == 0:
        msg = "notify_channels_empty"
        print(msg)
        return
    # 持久化到本地json文件
    persist(payload, trigger_time, rule_id)
    # 生成告警内容
    alert_content = content_gen(payload)

    for ch in notify_channels:
        send_func_name = "send_{}".format(ch.strip())
        has_func = hasattr(Send, send_func_name)

        if not has_func:
            msg = "[send_func_name_err][func_not_found_in_Send_class:{}]".format(send_func_name)
            print(msg)
            continue
        send_func = getattr(Send, send_func_name)
        send_func(alert_content)


def content_gen(payload):
    # 生成格式化告警内容
    text = ""
    event_obj = payload.get("event")

    rule_type = event_obj.get("is_prome_pull")
    type_str_m = {1: "prometheus", 0: "n9e"}
    rule_type = type_str_m.get(rule_type)

    text += "告警类型：{}\n".format(rule_type)

    rule_name = event_obj.get("rule_name")
    text += "规则名称：{}\n".format(rule_name)

    is_recovery = event_obj.get("is_recovery")
    is_recovery_str_m = {1: "已恢复", 0: "已触发"}
    is_recovery = is_recovery_str_m.get(is_recovery)
    text += "是否已恢复：{}\n".format(is_recovery)

    priority = event_obj.get("priority")
    text += "告警级别：{}\n".format(priority)

    trigger_time = event_obj.get("trigger_time")
    text += "触发时间：{}\n".format(time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(int(trigger_time))))

    readable_expression = event_obj.get("readable_expression")
    text += "可读表达式：{}\n".format(readable_expression)

    values = event_obj.get("values")
    text += "当前值：{}\n".format(values)

    tags = event_obj.get("tags")
    text += "标签组：{}\n".format(tags)

    print(text)
    return text


def persist(payload, trigger_time, rule_id):
    if not os.path.exists(LOCAL_EVENT_FILE_DIR):
        os.makedirs(LOCAL_EVENT_FILE_DIR)

    filename = '%d_%d' % (trigger_time, rule_id)
    filepath = os.path.join(LOCAL_EVENT_FILE_DIR, filename)
    with open(filepath, 'w') as f:
        f.write(json.dumps(payload, indent=4))


class Send(object):
    @classmethod
    def send_mail(cls, payload):
        users = payload.get("event").get("users")
        emails = [x.get("email") for x in users]
        if not emails:
            print("[emails_empty]")
            return
        recipients = emails

        message = MIMEText(mail_body, 'html', 'utf-8')
        message['From'] = mail_from
        message['To'] = ", ".join(recipients)
        message["Subject"] = "n9e alert"

        smtp = smtplib.SMTP_SSL(mail_host, mail_port)
        smtp.login(mail_user, mail_pass)
        smtp.sendmail(mail_from, recipients, message.as_string())
        smtp.close()

        print("send_mail_success")

    @classmethod
    def send_wecom(cls, payload):
        print("send_wecom")

    @classmethod
    def send_dingtalk(cls, payload):
        # TODO 钉钉发群信息需要群的webhook机器人 token，这个信息可以写在告警策略中的附加字段中
        dingtalk_api_url = "https://oapi.dingtalk.com/robot/send?access_token=xxxx"
        users = payload.get("event").get("users")
        atMobiles = [x.get("phone") for x in users]
        headers = {'Content-Type': 'application/json;charset=utf-8'}
        payload = {
            "msgtype": "text",
            "text": {
                "content": payload
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
