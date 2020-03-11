import React, { Component } from 'react';
import { InputNumber, Select, Input, Tag, Spin } from 'antd';
import _ from 'lodash';

interface Props {
  value: any,
  onChange: (values: any) => void,
  readOnly: boolean,
  notifyGroupLoading: boolean,
  notifyUserLoading: boolean,
  notifyGroupData: any[],
  notifyUserData: any[],
  fetchNotifyData: (options1?: any, options2?: any) => void,
}

const { Option } = Select;

export default class Actions extends Component<Props> {
  static checkActions = checkActions;

  static defaultValue = {
    converge: [3600, 1],
    notify_group: [],
    notify_user: [],
    callback: '',
  };

  static defaultProps = {
    readOnly: false,
    notifyGroupLoading: false,
    notifyUserLoading: false,
    notifyGroupData: [],
    notifyUserData: [],
  };

  handleConvergeChange = (index: number, val: number | undefined) => {
    const { value } = this.props;
    const valueClone = _.cloneDeep(value);
    const convergeValue = valueClone.converge;

    convergeValue[index] = index === 0 ? val * 60 : val;
    this.props.onChange({
      ...value,
      converge: convergeValue,
    });
  }

  handleNotifyGroupChange = (val: string) => {
    const { value } = this.props;
    this.props.onChange({
      ...value,
      notify_group: val,
    });
  }

  handleNotifyUserChange = (val: string) => {
    const { value } = this.props;
    this.props.onChange({
      ...value,
      notify_user: val,
    });
  }

  handleCallbackChange = (val: string) => {
    const { value } = this.props;

    this.props.onChange({
      ...value,
      callback: val,
    });
  }

  render() {
    const { readOnly, value, notifyGroupData, notifyUserData } = this.props;
    const { converge } = value;
    const errors = checkActions(null, this.props.value, _.noop) || {} as any;

    if (readOnly) {
      return (
        <div className="strategy-actions">
          <div> 在 {converge[0]} 分钟内, 最多报警 {converge[1]} 次 </div>
          <div>
            报警接收组: {_.map(value.notify_group, o => <Tag key={o}>{o}</Tag>)}
          </div>
          {
            value.callback ? <div>回调地址: {value.callback}</div> : null
          }
        </div>
      );
    }
    return (
      <div className="strategy-actions">
        <div className={!_.isEmpty(errors.converge) ? 'has-error' : undefined}>
          在
          <InputNumber
            style={{ marginLeft: 8 }}
            size="default"
            min={1}
            value={converge[0] / 60}
            onChange={(val) => { this.handleConvergeChange(0, val); }}
          />
          分钟内,
          最多报警
          <InputNumber
            style={{ marginLeft: 8 }}
            size="default"
            min={0}
            value={converge[1]}
            onChange={(val) => { this.handleConvergeChange(1, val); }}
            />
          次
          <div className="ant-form-explain">{errors.converge}</div>
        </div>
        <div>
          报警接收团队
        </div>
        <div className={errors.notifyGroup ? 'has-error' : undefined}>
          <Select
            showSearch
            mode="multiple"
            size="default"
            notFoundContent={this.props.notifyGroupLoading ? <Spin size="small" /> : null}
            defaultActiveFirstOption={false}
            filterOption={false}
            placeholder="报警接收团队"
            value={value.notify_group}
            onChange={this.handleNotifyGroupChange}
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
          <div className="ant-form-explain">{errors.notifyGroup}</div>
        </div>
        <div>
          报警接收人
        </div>
        <div className={errors.notifyGroup ? 'has-error' : undefined}>
          <Select
            showSearch
            mode="multiple"
            size="default"
            notFoundContent={this.props.notifyUserLoading ? <Spin size="small" /> : null}
            defaultActiveFirstOption={false}
            filterOption={false}
            placeholder="报警接收人"
            value={value.notify_user}
            onChange={this.handleNotifyUserChange}
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
          <div className="ant-form-explain">{errors.notifyUser}</div>
        </div>
        <div>
          通知我自己开发的系统（报警回调, 请确认是 IDC 内可访问的地址）
        </div>
        <div className={errors.callback ? 'has-error' : undefined}>
          <Input
            size="default"
            addonBefore="http://"
            value={value.callback}
            onChange={(e) => { this.handleCallbackChange(e.target.value); }}
          />
          <div className="ant-form-explain">{errors.callback}</div>
        </div>
      </div>
    );
  }
}

function checkActions(rule: any, value: any, callbackFunc: any) {
  const emptyErrorText = '不能为空';
  const { converge } = value;
  const errors: any = {
    converge: '',
    notifyGroup: '',
    callback: '',
  };
  let hasError = false;

  if (converge) {
    if (converge[0] === undefined) {
      errors.converge = [emptyErrorText, ''];
      hasError = true;
    } else if (converge[1] === undefined) {
      errors.converge = ['', emptyErrorText];
      hasError = true;
    }
  }

  if (hasError) {
    callbackFunc(JSON.stringify(errors));
    return errors;
  }
  callbackFunc();
  return undefined;
}
