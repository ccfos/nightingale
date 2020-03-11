import React, { Component } from 'react';
import _ from 'lodash';
import moment from 'moment';
import { Popover } from 'antd';
import * as config from '../config';
import { GraphDataInterface, CounterInterface } from '../interface';

interface Props {
  graphConfig: GraphDataInterface,
  counterList: CounterInterface[],
  children: React.ReactNode,
}

export default class Info extends Component<Props> {

  shouldComponentUpdate(nextProps: Props) {
    return !_.isEqual(nextProps, this.props);
  }

  getContent() {
    const { graphConfig, counterList } = this.props;
    const { unit, start, end } = graphConfig;
    const metricGroup = _.groupBy(counterList, 'metric');

    return (
      _.map(metricGroup, (metricGroupVals, groupName) => {
        const firstItem = metricGroupVals[0] || {};
        return (
          <ul className="graph-info" key={groupName}>
            <li>
              <span className="graph-info-key">指标:</span>
              <span className="graph-info-value">{groupName}</span>
            </li>
            <li>
              <span className="graph-info-key">采集周期:</span>
              <span className="graph-info-value">{firstItem.step ? `${firstItem.step} s` : '无'}</span>
            </li>
            <li>
              <span className="graph-info-key">时间范围:</span>
              <span className="graph-info-value">
                {moment(Number(start)).format(config.timeFormatMap.moment)}
                <span> - </span>
                {moment(Number(end)).format(config.timeFormatMap.moment)}
              </span>
            </li>
            {
              unit ?
                <li>
                  <span className="graph-info-key">单位:</span>
                  <span className="graph-info-value">{unit}</span>
                </li> : null
            }
          </ul>
        );
      })
    );
  }

  render() {
    return (
      <Popover
        trigger="click"
        content={this.getContent()}
        title="详情"
        placement="topLeft"
      >
        {this.props.children}
      </Popover>
    );
  }
}
