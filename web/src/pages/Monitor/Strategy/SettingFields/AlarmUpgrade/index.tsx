import React, { Component } from 'react';
import { InputNumber, Select, Spin, Checkbox } from 'antd';
import _ from 'lodash';

interface Props {
  notifyGroupLoading: boolean,
  notifyUserLoading: boolean,
  notifyGroupData: any[],
  notifyUserData: any[],
  value: any,
  onChange: (values: any) => void,
  readOnly: boolean,
  fetchNotifyData: (options1?: any, options2?: any) => void,
}

const { Option } = Select;

export default class AlarmUpgrade extends Component<Props> {
  static checkAlarmUpgrade = checkAlarmUpgrade;

  static defaultValue = {
    enabled: false,
    users: [],
    groups: [],
    duration: undefined,
    level: undefined,
  };

  static defaultProps = {
    readOnly: false,
    notifyGroupData: [],
    notifyUserData: [],
  };

  render() {
    const { readOnly, value, notifyGroupData, notifyUserData } = this.props;
    const errors = checkAlarmUpgrade(null, this.props.value, _.noop);

    if (readOnly) {
      return null;
    }
    return (
      <div className="strategy-alarm-upgrade">
        <div>
          <Checkbox
            checked={value.enabled}
            onChange={(e) => {
              this.props.onChange({
                ...value,
                enabled: e.target.checked,
              });
            }}
          >
            是否启动报警升级
          </Checkbox>
        </div>
        <div>
          持续
          <InputNumber
            min={0}
            style={{ margin: '0 8px' }}
            value={value.duration ? value.duration / 60 : undefined}
            onChange={(val) => {
              this.props.onChange({
                ...value,
                duration: val * 60,
              });
            }}
          />
          分钟，未处理或者未恢复的持续报警，将以
          <Select
            style={{ width: 100, margin: '0 8px' }}
            value={value.level}
            onChange={(val: number) => {
              this.props.onChange({
                ...value,
                level: val,
              });
            }}
          >
            <Option key="1" value={1}>一级报警</Option>
            <Option key="2" value={2}>二级报警</Option>
            <Option key="3" value={3}>三级报警</Option>
          </Select>
          发送给
        </div>
        <div>
          报警接收团队
        </div>
        <div className={errors.notify ? 'has-error' : undefined}>
          <Select
            showSearch
            mode="multiple"
            size="default"
            notFoundContent={this.props.notifyGroupLoading ? <Spin size="small" /> : null}
            defaultActiveFirstOption={false}
            filterOption={false}
            placeholder="报警接收团队"
            value={value.groups}
            onChange={(val: any) => {
              this.props.onChange({
                ...value,
                groups: val,
              });
            }}
            onSearch={(val) => {
              this.props.fetchNotifyData({ query: val });
            }}
          >
            {
              _.map(notifyGroupData, (item, i) => {
                return (
                  <Option key={i} value={item.id}>{item.name}</Option>
                );
              })
            }
          </Select>
          <div className="ant-form-explain">{errors.notify}</div>
        </div>
        <div>
          报警接收人
        </div>
        <div className={errors.notify ? 'has-error' : undefined}>
          <Select
            showSearch
            mode="multiple"
            size="default"
            notFoundContent={this.props.notifyUserLoading ? <Spin size="small" /> : null}
            defaultActiveFirstOption={false}
            filterOption={false}
            placeholder="报警接收人"
            value={value.users}
            onChange={(val: any) => {
              this.props.onChange({
                ...value,
                users: val,
              });
            }}
            onSearch={(val) => {
              this.props.fetchNotifyData(null, { query: val });
            }}
          >
            {
              _.map(notifyUserData, (item, i) => {
                return (
                  <Option key={i} value={item.id}>{item.username} {item.dispname} {item.phone} {item.email}</Option>
                );
              })
            }
          </Select>
          <div className="ant-form-explain">{errors.notify}</div>
        </div>
      </div>
    );
  }
}

function checkAlarmUpgrade(rule: any, value: any, callbackFunc: any) {
  const errors = {
    notify: '',
  };
  let hasError = false;

  if (value.enabled && _.isEmpty(value.users) && _.isEmpty(value.groups)) {
    hasError = true;
    errors.notify = '必须存在一个报警接收人或接收组';
  }

  if (hasError) {
    callbackFunc(JSON.stringify(errors));
  } else {
    callbackFunc();
  }
  return errors;
}
