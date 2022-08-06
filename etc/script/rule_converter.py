import json
import yaml

'''
将promtheus/vmalert的rule转换为n9e中的rule
支持k8s的rule configmap
'''

rule_file = 'rules.yaml'


def convert_interval(interval):
    if interval.endswith('s') or interval.endswith('S'):
        return int(interval[:-1])
    if interval.endswith('m') or interval.endswith('M'):
        return int(interval[:-1]) * 60
    if interval.endswith('h') or interval.endswith('H'):
        return int(interval[:-1]) * 60 * 60
    if interval.endswith('d') or interval.endswith('D'):
        return int(interval[:-1]) * 60 * 60 * 24
    return int(interval)


def convert_alert(rule, interval):
    name = rule['alert']
    prom_ql = rule['expr']
    if 'for' in rule:
        prom_for_duration = convert_interval(rule['for'])
    else:
        prom_for_duration = 0

    prom_eval_interval = convert_interval(interval)
    note = ''
    if 'annotations' in rule:
        for v in rule['annotations'].values():
            note = v
            break

    append_tags = []
    severity = 2
    if 'labels' in rule:
        for k, v in rule['labels'].items():
            if k != 'severity':
                append_tags.append('{}={}'.format(k, v))
                continue
            if v == 'critical':
                severity = 1
            elif v == 'info':
                severity = 3
            # elif v == 'warning':
            #     severity = 2


    n9e_alert_rule = {
        "name": name,
        "note": note,
        "severity": severity,
        "disabled": 0,
        "prom_for_duration": prom_for_duration,
        "prom_ql": prom_ql,
        "prom_eval_interval": prom_eval_interval,
        "enable_stime": "00:00",
        "enable_etime": "23:59",
        "enable_days_of_week": [
            "1",
            "2",
            "3",
            "4",
            "5",
            "6",
            "0"
        ],
        "enable_in_bg": 0,
        "notify_recovered": 1,
        "notify_channels": [],
        "notify_repeat_step": 60,
        "recover_duration": 0,
        "callbacks": [],
        "runbook_url": "",
        "append_tags": append_tags
    }
    return n9e_alert_rule


def convert_record(rule, interval):
    name = rule['record']
    prom_ql = rule['expr']
    prom_eval_interval = convert_interval(interval)
    note = ''
    append_tags = []
    if 'labels' in rule:
        for k, v in rule['labels'].items():
            append_tags.append('{}={}'.format(k, v))

    n9e_record_rule = {
        "name": name,
        "note": note,
        "disabled": 0,
        "prom_ql": prom_ql,
        "prom_eval_interval": prom_eval_interval,
        "append_tags": append_tags
    }
    return n9e_record_rule


'''
example of rule group file
---
groups:
- name: example
  rules:
  - alert: HighRequestLatency
    expr: job:request_latency_seconds:mean5m{job="myjob"} > 0.5
    for: 10m
    labels:
      severity: page
    annotations:
      summary: High request latency
'''
def deal_group(group):
    """
    parse single prometheus/vmalert rule group
    """
    alert_rules = []
    record_rules = []

    for rule_segment in group['groups']:
        if 'interval' in rule_segment:
            interval = rule_segment['interval']
        else:
            interval = '15s'
        for rule in rule_segment['rules']:
            if 'alert' in rule:
                alert_rules.append(convert_alert(rule, interval))
            else:
                record_rules.append(convert_record(rule, interval))

    return alert_rules, record_rules


'''
example of k8s rule configmap
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: rulefiles-0
data:
  etcdrules.yaml: |
    groups:
    - name: etcd
      rules:
      - alert: etcdInsufficientMembers
        annotations:
          message: 'etcd cluster "{{ $labels.job }}": insufficient members ({{ $value}}).'
        expr: sum(up{job=~".*etcd.*"} == bool 1) by (job) < ((count(up{job=~".*etcd.*"})
          by (job) + 1) / 2)
        for: 3m
        labels:
          severity: critical
'''
def deal_configmap(rule_configmap):
    """
    parse rule configmap from k8s
    """
    all_record_rules = []
    all_alert_rules = []
    for _, rule_group_str in rule_configmap['data'].items():
        rule_group = yaml.load(rule_group_str, Loader=yaml.FullLoader)
        alert_rules, record_rules = deal_group(rule_group)
        all_alert_rules.extend(alert_rules)
        all_record_rules.extend(record_rules)

    return all_alert_rules, all_record_rules


def main():
    with open(rule_file, 'r') as f:
        rule_config = yaml.load(f, Loader=yaml.FullLoader)
        
        # 如果文件是k8s中的configmap,使用下面的方法
        # alert_rules, record_rules = deal_configmap(rule_config)
        alert_rules, record_rules = deal_group(rule_config)

        with open("alert-rules.json", 'w') as fw:
            json.dump(alert_rules, fw, indent=2, ensure_ascii=False)

        with open("record-rules.json", 'w') as fw:
            json.dump(record_rules, fw, indent=2, ensure_ascii=False)


if __name__ == '__main__':
    main()
