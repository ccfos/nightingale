import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import _ from 'lodash';
import moment from 'moment';
import { Button, Select, DatePicker } from 'antd';
import * as config from '../config';
import * as util from '../util';
import { UpdateGrapDataInterface } from '../interface';

interface Props {
  refreshVisible: boolean,
  now: string,
  start: string,
  end: string,
  onChange: (updateGraphConfig: UpdateGrapDataInterface) => void,
}

export default class GlobalOperationbar extends Component<Props> {
  static defaultProps = {
    refreshVisible: true,
    now: '',
    start: '',
    end: '',
    onChange: _.noop,
  };

  state = {};

  handleRefresh = () => {
    // TODO: 如果用户选择的是 "其他" 时间，然后再点击 "刷新" 按钮，这时候 end 就会被强制更新到 now 了，这块有待考虑下怎么处理
    const { onChange, start, end } = this.props;
    const now = moment();
    const nowTs = now.format('x');
    let newStart = start;
    let newEnd = end;

    if (start && end) {
      newStart = (Number(nowTs) - Number(end)) + Number(start) + '';
      newEnd = nowTs;
    }

    onChange({
      now: nowTs,
      start: newStart,
      end: newEnd,
    });
  }

  handleTimeOptionChange = (val: string) => {
    const { onChange } = this.props;
    const now = this.props.now ? moment(Number(this.props.now)) : moment();
    const newNow = typeof now === 'string' ? now : now.clone().format('x');
    let start = this.props.start || now.clone().subtract(3600001, 'ms').format('x');
    let end = this.props.end || now.clone().format('x');

    if (val !== 'custom') {
      start = moment(Number(now)).subtract(Number(val), 'ms').format('x');
      end = moment(Number(now)).format('x');
    } else {
      start = moment(Number(start)).format('x');
      end = moment().format('x');
    }

    onChange({
      now: newNow,
      start,
      end,
    });
  }

  handleDateChange = (key: string, d: moment.Moment) => {
    let { start, end } = this.props;

    if (moment.isMoment(d)) {
      const ts = d.format('x');

      if (key === 'start') {
        start = ts;
      }
      if (key === 'end') {
        end = ts;
      }

      this.props.onChange({
        start,
        end,
      });
    }
  }

  render() {
    const { now, start, end } = this.props;
    let timeVal;

    if (now && start && end) {
      timeVal = now === end ? util.getTimeLabelVal(start, end, 'value') : 'custom';
    }

    const datePickerStartVal = start ? moment(Number(start)).format(config.timeFormatMap.moment) : null;
    const datePickerEndVal = end ? moment(Number(end)).format(config.timeFormatMap.moment) : null;

    return (
      <div className="global-operationbar-warp">
        {
          this.props.refreshVisible ?
            <Button onClick={this.handleRefresh} style={{ marginRight: 8 }}>
              <FormattedMessage id="graph.refresh" />
            </Button> : null
        }
        <span>
          <Select
            style={{ width: 80 }}
            value={timeVal}
            onChange={this.handleTimeOptionChange}
          >
            {
              _.map(config.time, (o) => {
                return (
                  <Select.Option key={o.value} value={o.value}>
                    <FormattedMessage id={o.label} />
                  </Select.Option>
                );
              })
            }
          </Select>
          {
            timeVal === 'custom' ?
              [
                <DatePicker
                  showTime
                  key="datePickerStart"
                  style={{
                    width: 175,
                    minWidth: 175,
                    marginLeft: 5,
                  }}
                  format={config.timeFormatMap.moment}
                  defaultValue={moment(datePickerStartVal)}
                  onOk={d => this.handleDateChange('start', d)}
                />,
                <span key="datePickerDivider" style={{ paddingLeft: 5, paddingRight: 5 }}>-</span>,
                <DatePicker
                  showTime
                  key="datePickerEnd"
                  style={{
                    width: 175,
                    minWidth: 175,
                  }}
                  format={config.timeFormatMap.moment}
                  defaultValue={moment(datePickerEndVal)}
                  onOk={d => this.handleDateChange('end', d)}
                />,
              ] : false
          }
        </span>
      </div>
    );
  }
}
