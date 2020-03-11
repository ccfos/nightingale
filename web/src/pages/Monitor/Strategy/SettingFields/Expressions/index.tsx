import React, { Component } from 'react';
import { Select, Tag } from 'antd';
import _ from 'lodash';
import Expression from './Expression';
import { defaultExpressionValue, commonPropDefaultValue } from './config';
import './style.less';

interface Props {
  metricError: string,
  value: any,
  alertDuration: number,
  readOnly: boolean,
  metrics: any[],
  onChange: (values: any) => void,
  renderHeader: (value: any) => React.ReactNode,
  renderFooter: (value: any) => React.ReactNode,
}

const { Option } = Select;

export default class Expressions extends Component<Props> {
  static defaultExpressionValue = defaultExpressionValue;

  static checkExpressions = checkExpressions;

  static defaultProps = {
    ...commonPropDefaultValue,
  };

  handleTypeChange = (type: string) => {
    const { value } = this.props;
    const valueClone = _.cloneDeep(value);

    if (type === 'normal') {
      this.props.onChange([valueClone[0]]);
    } else if (type === 'and') {
      valueClone.push(defaultExpressionValue);
      this.props.onChange(valueClone);
    }
  }

  handleExpressionChange = (index: number, val: any) => {
    const { value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    valueClone[index] = val;
    onChange(valueClone);
  }

  render() {
    const { alertDuration, value, readOnly, metrics, renderHeader, renderFooter } = this.props;
    const type = value[1] ? 'and' : 'normal';
    const errors = checkExpressions(null, value, _.noop) || [];

    return (
      <div className="strategy-expressions">
        {
          !readOnly &&
          <Select
            style={{ width: 80 }}
            size="default"
            value={type}
            onChange={this.handleTypeChange}
          >
            <Option value="normal">常用</Option>
            <Option value="and">与条件</Option>
          </Select>
        }
        <div>
          <Expression
            alertDuration={alertDuration}
            value={value[0] || {}}
            metricError={errors[0]}
            onChange={newValue => this.handleExpressionChange(0, newValue)}
            readOnly={readOnly}
            metrics={metrics}
            renderHeader={renderHeader}
            renderFooter={renderFooter}
          />
          {
            type === 'and' &&
            <div className="expressions-and">
              <div className="expressions-and-tagBorder" />
              <span className="expressions-and-tag">
                <Tag>与</Tag>
              </span>
              <Expression
                alertDuration={alertDuration}
                value={value[1] || {}}
                metricError={errors[1]}
                onChange={newValue => this.handleExpressionChange(1, newValue)}
                readOnly={readOnly}
                metrics={metrics}
                renderHeader={renderHeader}
                renderFooter={renderFooter}
              />
            </div>
          }
        </div>
      </div>
    );
  }
}

function checkExpressions(rule: any, value: any, callback: any) {
  let error0;
  let error1;
  const emptyErrorText = '不能为空';
  const samenameErrorText = '与条件, 不能选择相同的 metric';
  let hasError = false;

  _.each(value, (item, i: number) => {
    if (item.metric === '') {
      if (i === 0) {
        error0 = emptyErrorText;
        hasError = true;
      } else if (i === 1) {
        error1 = emptyErrorText;
        hasError = true;
      }
    } else if (i === 1 && item.metric === value[0].metric) {
      error0 = samenameErrorText;
      error1 = samenameErrorText;
      hasError = true;
    }
  });

  const errors = [error0, error1];

  if (hasError) {
    callback(JSON.stringify(errors));
    return [error0, error1];
  }
  callback();
  return undefined;
}
