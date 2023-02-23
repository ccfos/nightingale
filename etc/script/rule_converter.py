# coding=utf-8
import re
import os
import sys
import glob
import json
import yaml
import argparse

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


def convert_alert(rule, interval, ruletmpl):
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
            # expr 经常包含空格，跳过这个标签
            if k == 'expr' or  k == 'env' :
                continue
            if k != 'severity' and k != 'level' :
                append_tags.append('{}={}'.format(k, v))
                continue
            if v == 'critical':
                severity = 1
            elif v == 'info' or v == 'warning':
                severity = 3
            # elif v == 'warning':
            #     severity = 2
    if len(ruletmpl['tags']) >= 1:
        for k, v in ruletmpl['tags'].items():
            append_tags.append('{}={}'.format(k, v))
    
    disabled = 1
    if ruletmpl['enable']:
        disabled = 0


    n9e_alert_rule = {
        "name": name,
        "note": note,
        "severity": severity,
        "disabled": disabled,
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


def convert_record(rule, interval, ruletmpl):
    name = rule['record']
    prom_ql = rule['expr']
    prom_eval_interval = convert_interval(interval)
    note = ''
    append_tags = []
    if 'labels' in rule:
        for k, v in rule['labels'].items():
            # expr 经常包含空格，跳过这个标签
            if k == 'expr':
                continue
            append_tags.append('{}={}'.format(k, v))

    if len(ruletmpl['tags']) >= 1:
        for k, v in ruletmpl['tags'].items():
            append_tags.append('{}={}'.format(k, v))
    # record flag
    append_tags.append('{}={}'.format("record", "True"))   
    disabled = 1
    if ruletmpl['enable'] :
        disabled = 0

    n9e_record_rule = {
        "name": name,
        "note": note,
        "disabled": disabled,
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
def deal_group(group,ruletmpl):
    """
    parse single prometheus/vmalert rule group
    """
    alert_rules = []
    record_rules = []
    if not group.has_key('groups'):
        return [], []
    for rule_segment in group['groups']:
        if 'interval' in rule_segment:
            interval = rule_segment['interval']
        else:
            interval = '15s'
        if rule_segment['rules'] is None:
            continue
        for rule in rule_segment['rules']:
            if 'alert' in rule:
                alert_rules.append(convert_alert(rule, interval,ruletmpl))
            else:
                record_rules.append(convert_record(rule, interval,ruletmpl))

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

def findFiles(args):
    """
    find rules files
    """
    rule_files = []
    dir_list = []
    if args.files is not None:
        rule_files.extend(args.files.split(","))
    if args.directories is not None:
        dir_list.extend(args.directories.split(","))
    if dir_list != []:
        for dir in dir_list:
            abs_directory = os.path.abspath(dir)
            pattern = os.path.join(dir, "*.y*ml")
            files = glob.glob(pattern)
            abs_files = [os.path.abspath(f) for f in files]
            rule_files.extend(abs_files)

    if rule_files == []:
        print("没有找到指定文件")
        sys.exit()
    return rule_files

def converter(rule_files,alert_file,record_file,append,ruletmpl):
    """
    转换规则
    """
    mode = 'w'
    if append:
        mode = 'aw'
    all_alert_rules = []
    all_record_rules = []

    for file in rule_files:
        with open(file, 'r') as f:
            rule_config = yaml.load(f, Loader=yaml.FullLoader)
            
            # 如果文件是k8s中的configmap,使用下面的方法
            # alert_rules, record_rules = deal_configmap(rule_config)
            alert_rules, record_rules = deal_group(rule_config,ruletmpl)
            all_alert_rules.extend(alert_rules)
            all_record_rules.extend(record_rules)
    
    with open(alert_file, mode) as fw:
        json.dump(all_alert_rules, fw, indent=2, ensure_ascii=False)

    with open(record_file, mode) as fw:
        json.dump(all_record_rules, fw, indent=2, ensure_ascii=False)
            

def main():
    parser = argparse.ArgumentParser(description='将promtheus/vmalert的rule转换为n9e中的rule')
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('-f', '--files', type=str, help='指定文件，多个文件用逗号分隔')
    group.add_argument('-d', '--directories', type=str, help='指定目录，多个目录用逗号分隔')
    parser.add_argument('-a', '--append', action='store_true', help='追加写入模式')
    parser.add_argument('-t', '--tags', type=str, help='指定标签，多个标签用逗号分隔，标签格式必须符合要求`key=value`且不重复')
    parser.add_argument('-o', '--outfile', type=str, help='指定输出文件名的前缀，文件名不能包含特殊字符')
    parser.add_argument('--enable', action='store_true', help='启用规则,默认为不启用')
    parser.set_defaults(append=False)

    args = parser.parse_args()

    if args.outfile:
        alert_rules_file = args.outfile+"_alert.json"
        record_rules_file = args.outfile+"_record.json"
    else:
        alert_rules_file = "alert-rules.json"
        record_rules_file = "record-rules.json"
    
    if args.files or args.directories:
        # 生成rule文件列表
        rule_file = findFiles(args)
    ruletmpl = { 'tags': '', 'enable': 0}
    if args.enable:
        ruletmpl['enable']= 1
    if args.tags is not None:
        pattern = r"\b(\w+)=(\w+)\b"
        matches = re.findall(pattern, args.tags)
        tags = {}
        for match in matches:
            key, value = match
            tags[key] = value
        ruletmpl['tags']= tags

    print('Inputfile:', rule_file)
    print('Outputfile: ', alert_rules_file, record_rules_file)
    print('Append:', args.append)
    print('Tags:', args.tags)

    converter(rule_file,alert_rules_file,record_rules_file,args.append,ruletmpl)


if __name__ == '__main__':
    main()
