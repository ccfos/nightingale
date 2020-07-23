/* eslint-disable no-template-curly-in-string */
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Card, Select, InputNumber } from 'antd';
import _ from 'lodash';
import { FormattedMessage, injectIntl } from 'react-intl';
import { funcMap, defaultExpressionValue, commonPropTypes, commonPropDefaultValue } from './config';

const { Option } = Select;

class Expression extends Component {
  static propTypes = {
    ...commonPropTypes,
    value: PropTypes.object,
    metricError: PropTypes.string,
  };

  static defaultProps = {
    ...commonPropDefaultValue,
    value: defaultExpressionValue,
    metricError: '',
  };

  handleMetricChange = (val) => {
    const { value, onChange } = this.props;

    onChange({
      ...value,
      metric: val,
    });
  }

  handleFuncChange = (val) => {
    const { value, onChange } = this.props;
    const currentFuncDefaultValue = _.get(funcMap[val], 'defaultValue', []);

    onChange({
      ...value,
      func: val,
      params: currentFuncDefaultValue,
    });
  }

  handleParamsChange = (index, val) => {
    const { value, onChange } = this.props;
    const currentFuncDefaultValue = _.get(funcMap[value.func], 'defaultValue', []);
    const { params = [] } = value;

    if (val === '' || val === undefined) {
      params[index] = currentFuncDefaultValue[index];
    } else {
      params[index] = val;
    }

    onChange({
      ...value,
      params,
    });
  }

  handleEoptChange = (val) => {
    const { value, onChange } = this.props;

    onChange({
      ...value,
      eopt: val,
    });
  }

  handleThresholdChange = (val) => {
    const { value, onChange } = this.props;
    let newVal = val;

    if (val === '' || val === undefined) {
      newVal = defaultExpressionValue.threshold;
    }

    onChange({
      ...value,
      threshold: newVal,
    });
  }

  renderPreview(readOnly: boolean) {
    const { value, alertDuration } = this.props;
    const { metric, func, eopt, threshold } = value;
    const { params = [] } = value;
    const meaningKey = this.props.intl.locale === 'en' ? 'meaningEn' : 'meaning';
    const str = _.get(funcMap[func], meaningKey, '');
    const index1 = str.indexOf('$n');
    const index2 = str.indexOf('$m');
    const index3 = str.lastIndexOf('$v');
    const nPrefix = str.substring(0, index1);
    const vPostfix = str.substring(index3 + 2);
    let mVal;
    if (
      func === 'c_avg_rate_abs'
      || func === 'c_avg_rate'
      || func === 'c_avg_abs'
      || func === 'c_avg'
    ) {
      mVal = params[0] !== 1 ? params[0] / 86400 : 1;
    } else {
      mVal = params[0] || 1;
    }
    let n = <strong style={{ color: '#2DB7F5' }}>{alertDuration}</strong>;
    const m = <strong style={{ color: '#FFB727' }}>{mVal}</strong>;
    // eslint-disable-next-line prefer-template,no-template-curly-in-string
    const v = <strong style={{ color: '#FF6F27' }}>{eopt && threshold !== undefined ? eopt + ' ' + threshold : '${v}'}</strong>;

    if (['diff', 'pdiff'].indexOf(func) > -1) {
      n = <strong style={{ color: '#2DB7F5' }}>{alertDuration}</strong>;
    }

    let previewNode = (
      <span>
        {nPrefix}
        {n}
      </span>
    );

    let nPostfix = str.substring(index1 + 2);
    if (index2 > -1) {
      const mPrefix = str.substring(index1 + 2, index2);
      previewNode = (
        <span>
          {previewNode}
          {mPrefix}
          {m}
        </span>
      );
      nPostfix = str.substring(index2 + 2);
    }

    if (func !== 'nodata' && func !== 'stddev') {
      // eslint-disable-next-line no-underscore-dangle
      const _index = index2 > -1 ? index2 : index1;
      const vPrefix = str.substring(_index + 2, index3);
      previewNode = (
        <span>
          {previewNode}
          {vPrefix}
          {v}
          {vPostfix}
        </span>
      );
    } else {
      // const nPostfix = str.substring(index1 + 2);
      previewNode = (
        <span>
          {previewNode}
          {nPostfix}
        </span>
      );
    }
    return (
      <div>
        { !readOnly && <span style={{ color: '#999' }}><FormattedMessage id="stra.preview" />：</span> }
        <span style={{ paddingRight: 5 }}>{metric || '${metric}' }</span>
        { previewNode }
      </div>
    );
  }

  renderFuncParams(i) {
    const { value } = this.props;
    const { func, params = [] } = value;
    const minnum = ['diff', 'pdiff'].indexOf(func) > -1 ? 2 : 1;
    // const maxnum = func === 'nodata' ? 7200 : 60;
    let val = _.toNumber(params[i]);

    if (i === 0) {
      if (
        func === 'c_avg_rate_abs'
        || func === 'c_avg_rate'
        || func === 'c_avg_abs'
        || func === 'c_avg'
      ) {
        // 相对天数
        val = _.toString(params[i] !== 1 ? params[i] : 86400);
        return (
          <Select
            style={{ display: 'inline-block', width: 80, marginRight: 8 }}
            value={val}
            onChange={(newVal) => { this.handleParamsChange(i, _.toNumber(newVal)); }}
          >
            <Option value="86400">1</Option>
            <Option value="604800">7</Option>
          </Select>
        );
      }
      if (func === 'happen' || func === 'ndiff' || func === 'stddev') {
        // 发生次数
        return (
          <InputNumber
            key={i}
            value={val}
            min={minnum}
            max={_.toNumber(params[0])}
            style={{ display: 'inline-block', marginRight: 8 }}
            onChange={(newVal) => { this.handleParamsChange(i, newVal); }}
          />
        );
      }
      return <span>不是合法的 param</span>;
    }
    return <span>不是合法的 param</span>;
  }

  renderParams() {
    const { value } = this.props;

    if (value.func === 'canary') {
      return null;
    }
    return (
      <div style={{ marginTop: 5 }}>
        {
          // render params
          _.map(_.get(funcMap[value.func], 'params', []), (o, i) => {
            return (
              <div key={o} style={{ display: 'inline-block', verticalAlign: 'top' }}>
                <span style={{ color: i === 0 ? '#FFB727' : '#FFB727' }}>{o}</span>
                <span style={{ marginRight: 8, marginLeft: 2 }}>:</span>
                { this.renderFuncParams(i) }
              </div>
            );
          })
        }
        {
          // render value
          value.func !== 'nodata' && value.func !== 'stddev' && // nodata 不需要填值
          <div style={{ display: 'inline-block' }}>
            <div style={{ display: 'inline-block', verticalAlign: 'top' }}>
              <span style={{ color: '#FF6F27' }}>v</span>
              <span style={{ marginRight: 8, marginLeft: 2 }}>:</span>
              <Select
                size="default"
                style={{ width: 70 }}
                value={value.eopt}
                onChange={this.handleEoptChange}>
                <Option value="=">=</Option>
                <Option value=">">&gt;</Option>
                <Option value=">=">&gt;=</Option>
                <Option value="<">&lt;</Option>
                <Option value="<=">&lt;=</Option>
                <Option value="!=">!=</Option>
              </Select>
            </div>
            <div style={{ display: 'inline-block', marginLeft: 10 }}>
              <InputNumber
                size="default"
                step={0.01}
                value={value.threshold}
                onChange={this.handleThresholdChange}
              />
            </div>
          </div>
        }
      </div>
    );
  }

  render() {
    const { value, metrics, renderHeader, renderFooter, metricError } = this.props;

    return (
      <Card
        bodyStyle={{ padding: 10 }}
        style={{ marginTop: 10 }}
      >
        <div className="expression-headerExtra">
          {renderHeader(value)}
        </div>
        <div className="expression-content">
          <div>
            <div className={metricError && 'has-error'} style={{ display: 'inline-block', verticalAlign: 'top' }}>
              <Select
                mode="combobox"
                notFoundContent=""
                size="default"
                style={{ width: 250 }}
                placeholder="Metric name"
                defaultActiveFirstOption={false}
                dropdownMatchSelectWidth={false}
                showSearch
                value={value.metric}
                onChange={this.handleMetricChange}>
                {
                  _.map(metrics, item => <Option key={item} value={item}>{item}</Option>)
                }
              </Select>
              <div className="ant-form-explain">{metricError}</div>
            </div>
            <Select
              style={{ width: 220, marginLeft: 10 }}
              size="default"
              value={value.func}
              onChange={this.handleFuncChange}>
              {
                _.map(funcMap, (val, key) => {
                  return <Option key={key} value={key}>{val.label} - {key}</Option>;
                })
              }
            </Select>
          </div>
          {this.renderParams()}
        </div>
        { this.renderPreview() }
        {
          value.func === 'all' ?
            <div style={{ color: '#f50', lineHeight: 1 }}><FormattedMessage id="stra.preview.all.help" /></div> : null
        }
        <div className="expression-footerExtra">
          {renderFooter(value)}
        </div>
      </Card>
    );
  }
}

export default injectIntl(Expression);
