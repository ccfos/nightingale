import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import update from 'react-addons-update';
import _ from 'lodash';
import moment from 'moment';
import { Icon, Button, Select, Checkbox, Tooltip, DatePicker } from 'antd';
import * as config from '../config';
import * as util from '../util';
import Tagkv from './Tagkv';
import Comparison from './Comparison';
import { GraphDataInterface, GraphDataChangeFunc } from '../interface';

interface Props {
  data: GraphDataInterface,
  onChange: GraphDataChangeFunc,
}

const { Option } = Select;

class GraphConfigInner extends Component<Props & WrappedComponentProps> {
  refresh = () => {
    // TODO 如果用户选择的是 "自定义" 时间，然后再点击 "刷新" 按钮，这时候 end 就会被强制更新到 now 了，这块有待考虑下怎么处理
    const { data, onChange } = this.props;
    const now = moment();
    const start = (Number(now.format('x')) - Number(data.end)) + Number(data.start) + '';
    const end = now.format('x');

    onChange('update', data.id, {
      start, end, now: end,
    });
  }

  timeOptionChange = (key: string) => {
    const { data, onChange } = this.props;
    const now = moment();
    let { start, end } = data;

    if (key !== 'custom') {
      start = now.clone().subtract(Number(key), 'ms').format('x');
      end = now.format('x');
    } else {
      start = moment(Number(start)).format('x');
      end = moment().format('x');
    }

    onChange('update', data.id, {
      start, end, now: end,
    });
  }

  dateChange(key: string, d: moment.Moment | null) {
    const { data, onChange } = this.props;
    let { start, end } = data;

    if (moment.isMoment(d)) {
      const ts = d.format('x');

      if (key === 'start') {
        start = ts;
      }
      if (key === 'end') {
        end = ts;
      }

      onChange('update', data.id, {
        start, end,
      });
    }
  }

  handleAggrFuncChange = (val: string) => {
    const { data, onChange } = this.props;
    onChange('update', data.id, {
      metrics: [{
        ...data.metrics[0],
        aggrFunc: val,
      }],
    });
  }

  handleComparisonChange = (values: any) => {
    const { data, onChange } = this.props;
    onChange('update', data.id, {
      start: values.start,
      end: values.end,
      now: values.now,
      comparison: values.comparison,
      relativeTimeComparison: values.relativeTimeComparison,
      comparisonOptions: values.comparisonOptions,
    });
  }

  handleconsolFuncChange = (val: string) => {
    const { data, onChange } = this.props;
    onChange('update', data.id, {
      metrics: [{
        ...data.metrics[0],
        consolFunc: val,
      }],
    });
  }

  legendChange = (e: any) => {
    const { data, onChange } = this.props;
    onChange('update', data.id, {
      legend: e.target.checked,
    });
  }

  sharedChange = (e: any) => {
    const { data, onChange } = this.props;
    onChange('update', data.id, {
      shared: e.target.checked,
    });
  }

  tagkvChange = (tagk: string, tagv: string[]) => {
    const { data, onChange } = this.props;
    const { metrics } = data;
    const firstMetric = metrics[0];
    const currentTagIndex = _.findIndex(firstMetric.selectedTagkv, { tagk });
    let selectedTagkv = [];
    let { selectedEndpoint } = firstMetric;

    if (currentTagIndex > -1) {
      if (!tagv.length) { // 删除
        selectedTagkv = update(firstMetric.selectedTagkv, {
          $splice: [
            [currentTagIndex, 1],
          ],
        });
      } else { // 修改
        selectedTagkv = update(firstMetric.selectedTagkv, {
          $splice: [
            [currentTagIndex, 1, {
              tagk, tagv,
            }],
          ],
        });
      }
    } else { // 新增
      // eslint-disable-next-line no-lonely-if
      if (tagv.length) {
        selectedTagkv = update(firstMetric.selectedTagkv, {
          $push: [{
            tagk, tagv,
          }],
        });
      } else {
        // eslint-disable-next-line prefer-destructuring
        selectedTagkv = firstMetric.selectedTagkv;
      }
    }

    if (tagk === 'endpoint') {
      selectedEndpoint = tagv;
    }

    onChange('update', data.id, {
      metrics: [{
        ...data.metrics[0],
        selectedTagkv,
        selectedEndpoint,
      }],
    });
  }

  renderTagkv() {
    const { metrics } = this.props.data;
    if (metrics.length === 1) {
      const firstMetric = metrics[0] || {};
      return (
        <div className="graph-config-inner-item">
          <Tagkv data={firstMetric.tagkv} selectedTagkv={firstMetric.selectedTagkv}
            onChange={this.tagkvChange}
            renderItem={(tagk, tagv, selectedTagv) => {
              return (
                <Button size="small" type="ghost" style={{ marginRight: 10 }}>
                  {
                    _.size(selectedTagv) !== 0 ? tagk + '(' + _.size(selectedTagv) + ')' : tagk
                  }
                  <Icon type="down" />
                </Button>
              );
            }} />
        </div>
      );
    }
    return null;
  }

  render() {
    const { data, onChange } = this.props;
    const { now, start, end, comparison } = data;
    const timeLabel = now === end ? util.getTimeLabelVal(start, end, 'label') : '其他';
    const timeVal = now === end ? util.getTimeLabelVal(start, end, 'value') : 'custom';
    const datePickerStartVal = moment(Number(start)).format(config.timeFormatMap.moment);
    const datePickerEndVal = moment(Number(end)).format(config.timeFormatMap.moment);
    const aggrGroupOptions = _.map(_.get(data.metrics, '[0].tagkv'), item => ({ label: item.tagk, value: item.tagk }));

    return (
      <div className="graph-config-inner">
        <div className="graph-config-inner-item">
          <Button size="small" type="ghost" onClick={this.refresh}>
            <FormattedMessage id="graph.refresh" />
          </Button>
        </div>
        <div className="graph-config-inner-item">
          <Select
            size="small"
            style={{ width: 70 }}
            value={this.props.intl.formatMessage({ id: timeLabel })}
            onChange={this.timeOptionChange}
          >
            {
              _.map(config.time, (o) => {
                return <Option key={o.value} value={o.value}>{this.props.intl.formatMessage({ id: o.label })}</Option>;
              })
            }
          </Select>
          {
            timeVal === 'custom' ?
              <span style={{
                display: 'inline-block',
                paddingLeft: 10,
                lineHeight: '22px',
                verticalAlign: 'top',
              }}>
                <DatePicker
                  showTime
                  size="small"
                  key="datePickerStart"
                  style={{
                    width: 165,
                    minWidth: 165,
                  }}
                  format={config.timeFormatMap.moment}
                  defaultValue={moment(datePickerStartVal)}
                  onOk={d => this.dateChange('start', d)}
                />
                <span key="datePickerDivider" style={{ paddingLeft: 5, paddingRight: 5 }}>-</span>
                <DatePicker
                  showTime
                  size="small"
                  key="datePickerEnd"
                  style={{
                    width: 165,
                    minWidth: 165,
                  }}
                  format={config.timeFormatMap.moment}
                  defaultValue={moment(datePickerEndVal)}
                  onOk={d => this.dateChange('end', d)}
                />
              </span> : false
          }
        </div>
        <div className="graph-config-inner-item">
          <FormattedMessage id="graph.config.aggr" />：
          <Select
            allowClear
            size="small"
            style={{ width: 85 }}
            placeholder="无"
            value={_.get(data.metrics, '[0].aggrFunc')}
            onChange={this.handleAggrFuncChange}
          >
            <Option value="sum"><FormattedMessage id="graph.config.aggr.sum" /></Option>
            <Option value="avg"><FormattedMessage id="graph.config.aggr.avg" /></Option>
            <Option value="max"><FormattedMessage id="graph.config.aggr.max" /></Option>
            <Option value="min"><FormattedMessage id="graph.config.aggr.min" /></Option>
          </Select>
        </div>
        {
          _.get(data.metrics, '[0].aggrFunc') ?
            <div className="graph-config-inner-item">
              <Tooltip title="按照某个 tag 聚合出多条曲线">
                <span><FormattedMessage id="graph.config.aggr.group" />：</span>
              </Tooltip>
              <Select
                mode="multiple"
                size="small"
                style={{ minWidth: 60 }}
                dropdownMatchSelectWidth={false}
                value={_.get(data.metrics, '[0].aggrGroup', [])}
                onChange={(val: string[]) => {
                  onChange('update', data.id, {
                    metrics: [{
                      ...data.metrics[0],
                      aggrGroup: val,
                    }],
                  });
                }}
              >
                {
                  _.map(aggrGroupOptions, o => <Option key={o.value} value={o.value}>{o.label}</Option>)
                }
              </Select>
            </div> : null
        }
        <div className="graph-config-inner-item">
          <FormattedMessage id="graph.config.comparison" />：
          <Comparison
            comparison={comparison}
            relativeTimeComparison={data.relativeTimeComparison}
            comparisonOptions={data.comparisonOptions}
            graphConfig={data}
            onChange={this.handleComparisonChange}
          />
          <input
            style={{
              position: 'fixed',
              left: -10000,
            }}
            id={`hiddenInput${data.id}`}
          />
        </div>
        <div className="graph-config-inner-item">
          <Checkbox checked={!!data.legend} onChange={this.legendChange}>
            Legend
          </Checkbox>
        </div>
        <div className="graph-config-inner-item">
          <Checkbox checked={!!data.shared} onChange={this.sharedChange}>
            Multi
          </Checkbox>
        </div>
        {this.renderTagkv()}
      </div>
    );
  }
}

export default injectIntl(GraphConfigInner);
