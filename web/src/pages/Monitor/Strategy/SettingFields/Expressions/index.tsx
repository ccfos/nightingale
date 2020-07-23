/* eslint-disable no-use-before-define */
import React, { Component } from 'react';
import { Select, Tag } from 'antd';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';
import Expression from './Expression';
import { defaultExpressionValue, commonPropTypes, commonPropDefaultValue } from './config';
import './style.less';

const { Option } = Select;

export default class Expressions extends Component {
  static defaultExpressionValue = defaultExpressionValue;

  static checkExpressions = checkExpressions;

  static propTypes = {
    ...commonPropTypes,
  };

  static defaultProps = {
    ...commonPropDefaultValue,
  };

  handleTypeChange = (type) => {
    const { value } = this.props;
    const valueClone = _.cloneDeep(value);

    if (type === 'normal') {
      this.props.onChange([valueClone[0]]);
    } else if (type === 'and') {
      valueClone.push(defaultExpressionValue);
      this.props.onChange(valueClone);
    }
  }

  handleExpressionChange = (index, val) => {
    const { value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    valueClone[index] = val;
    onChange(valueClone);
  }

  render() {
    const { alertDuration, value, readOnly, metrics, renderHeader, renderFooter } = this.props;
    const type = value[1] ? 'and' : 'normal';
    // eslint-disable-next-line no-use-before-define
    const errors = checkExpressions(null, value, _.noop) || [];

    return (
      <div className="strategy-expressions">
        {
          !readOnly &&
          <Select
            style={{ width: 90 }}
            size="default"
            value={type}
            onChange={this.handleTypeChange}
          >
            <Option value="normal"><FormattedMessage id="stra.trigger.normal" /></Option>
            <Option value="and"><FormattedMessage id="stra.trigger.and" /></Option>
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
                <Tag><FormattedMessage id="stra.trigger.and" /></Tag>
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

function checkExpressions(rule, value, callback) {
  let error0;
  let error1;
  const emptyErrorText = 'is required';
  const samenameErrorText = 'Cannot select the same metric';
  let hasError = false;

  _.each(value, (item, i) => {
    if (item.metric === '') {
      if (i === 0) {
        error0 = emptyErrorText;
        hasError = true;
      } else if (i === 1) {
        error1 = emptyErrorText;
        hasError = true;
      }
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
