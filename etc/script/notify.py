#!/usr/bin/python
# -*- coding: UTF-8 -*-
import sys
import json
import os
import smtplib
import time
import requests
from email.mime.text import MIMEText
from email.header import Header

reload(sys)                      # reload 才能调用 setdefaultencoding 方法
sys.setdefaultencoding('utf-8')  # 设置 'utf-8'

# 希望的demo实现效果：
# 1. 从stdin拿到告警信息之后，格式化为一个有缩进的json写入一个临时文件
# 2. 文件路径和名字是.alerts/${timestamp}_${ruleid}
# 3. 调用SMTP服务器发送告警，微信、钉钉、飞书、slack、jira、短信、电话等等留给社区实现

# 脚本二开指南
# 1. 可以根据下面的TEST_ALERT_JSON 中的结构修改脚本发送逻辑，定制化告警格式格式如下
"""
[告警类型：prometheus]
[规则名称：a]
[是否已恢复：已触发]
[告警级别：1]
[触发时间：2021-07-02 16:05:14]
[可读表达式：go_goroutines>0]
[当前值：[vector={__name__="go_goroutines", instance="localhost:9090", job="prometheus"}]: [value=33.000000]]
[标签组：instance=localhost:9090 job=prometheus]
"""
# 2. 每个告警会以json文件的格式存储在LOCAL_EVENT_FILE_DIR 下面，文件名为 filename = '%d_%d_%d' % (rule_id, event_id, trigger_time)
# 3. 告警通道需要自行定义Send类中的send_xxx同名方法，反射调用：举例 event.notify_channels = [qq dingding] 则需要Send类中 有 send_qq send_dingding方法
# 4. im发群信息，比如钉钉发群信息需要群的webhook机器人 token，这个信息可以在user的contacts map中，各个send_方法处理即可
# 5. 用户创建一个虚拟的用户保存上述im群 的机器人token信息 user的contacts map中

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

# dingding 群机器人token 配置字段
DINGTALK_ROBOT_TOKEN_NAME = "dingtalk_robot_token"
DINGTALK_API = "https://oapi.dingtalk.com/robot/send"

WECOM_ROBOT_TOKEN_NAME = "wecom_robot_token"
WECOM_API = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send"

# stdin 告警json实例
TEST_ALERT_JSON = {
    "event": {
        "alert_duration": 10,
        "notify_channels": "dingtalk",
        "res_classpaths": "",
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
        "res_ident": "",
        "rule_name": "a",
        "is_prome_pull": 1,
        "notify_users": "1",
        "notify_groups": "",
        "runbook_url": "",
        "values": "[vector={__name__=\"go_goroutines\", instance=\"localhost:9090\", job=\"prometheus\"}]: [value=33.000000]",
        "readable_expression": "go_goroutines>0",
        "notify_user_objs": None,
        "is_recovery": 0,
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
    alert_content = content_gen(payload)

    for ch in notify_channels:
        send_func_name = "send_{}".format(ch.strip())
        has_func = hasattr(Send, send_func_name)

        if not has_func:
            msg = "[send_func_name_err][func_not_found_in_Send_class:{}]".format(send_func_name)
            print(msg)
            continue
        send_func = getattr(Send, send_func_name)
        send_func(alert_content, payload)


def content_gen(payload):
    # 生成格式化告警内容
    text = ""
    event_obj = payload.get("event")

    rule_type = event_obj.get("is_prome_pull")
    type_str_m = {1: "prometheus", 0: "n9e"}
    rule_type = type_str_m.get(rule_type)

    text += "[告警类型：{}]\n".format(rule_type)

    rule_name = event_obj.get("rule_name")
    text += "[规则名称：{}]\n".format(rule_name)

    is_recovery = event_obj.get("is_recovery")
    is_recovery_str_m = {1: "已恢复", 0: "已触发"}
    is_recovery = is_recovery_str_m.get(is_recovery)
    text += "[是否已恢复：{}]\n".format(is_recovery)

    priority = event_obj.get("priority")
    text += "[告警级别：{}]\n".format(priority)

    trigger_time = event_obj.get("trigger_time")
    text += "[触发时间：{}]\n".format(time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(int(trigger_time))))

    readable_expression = event_obj.get("readable_expression")
    text += "[可读表达式：{}]\n".format(readable_expression)

    values = event_obj.get("values")
    text += "[当前值：{}]\n".format(values)

    tags = event_obj.get("tags")
    text += "[标签组：{}]\n".format(tags)

    print(text)
    return text


def persist(payload, rule_id, event_id, trigger_time):
    if not os.path.exists(LOCAL_EVENT_FILE_DIR):
        os.makedirs(LOCAL_EVENT_FILE_DIR)

    filename = '%d_%d_%d' % (rule_id, event_id, trigger_time)
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
    def send_wecom(cls, alert_content, payload):
        users = payload.get("users")

        for u in users:
            contacts = u.get("contacts")
            wecom_robot_token = contacts.get(WECOM_ROBOT_TOKEN_NAME, "")

            if wecom_robot_token == "":
                continue

            wecom_api_url = "{}?key={}".format(WECOM_API, wecom_robot_token)
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

            dingtalk_robot_token = contacts.get(DINGTALK_ROBOT_TOKEN_NAME, "")

            if dingtalk_robot_token == "":
                print("dingtalk_robot_token_not_found")
                continue

            dingtalk_api_url = "{}?access_token={}".format(DINGTALK_API, dingtalk_robot_token)
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
