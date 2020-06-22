import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import { Row, Col, Icon, Dropdown, Menu } from 'antd';
import _ from 'lodash';
import Graph, { GraphConfig, Info } from '@cpts/Graph';
import request from '@common/request';
import api from '@common/api';
import { prefixCls } from './config';
import SubscribeModal from './SubscribeModal';
import { normalizeGraphData } from './utils';
import { GraphData, UpdateType, GraphId } from './interface';

interface Props {
  value: GraphData[],
  onChange: (type: UpdateType, id: GraphId, updateConf?: GraphData, cbk?: () => void) => void,
  onGraphConfigSubmit: (type: UpdateType, data: GraphData, id: GraphId) => void,
}

export default class Graphs extends Component<Props> {
  graphConfigForm: any;
  static defaultProps = {
    value: [],
    onChange: () => {},
    onGraphConfigSubmit: () => {},
  };

  handleSubscribeGraph = (graphData: GraphData) => {
    const data = normalizeGraphData(graphData);
    const configs = JSON.stringify(data);
    SubscribeModal({
      configsList: [configs],
    });
  }

  handleShareGraph = (graphData: GraphData) => {
    const data = normalizeGraphData(graphData);
    const configsList = [{
      configs: JSON.stringify(data),
    }];
    request(api.tmpchart, {
      method: 'POST',
      body: JSON.stringify(configsList),
    }).then((res) => {
      window.open(`/#/monitor/tmpchart?ids=${_.join(res, ',')}`, '_blank');
    });
  }

  render() {
    const { value, onChange } = this.props;
    return (
      <div>
        <Row gutter={10} className={`${prefixCls}-graphs`}>
          {
            _.map(value, (o) => {
              return (
                <Col span={24} key={o.id}>
                  <div className={`${prefixCls}-graph`}>
                    <Graph
                      data={o}
                      onChange={onChange}
                      extraRender={(graph: GraphData) => {
                        return [
                          <span className="graph-operationbar-item" key="info">
                            <Info
                              graphConfig={graph.getGraphConfig(graph.props.data)}
                              counterList={graph.counterList}
                            >
                              <Icon type="info-circle-o" />
                            </Info>
                          </span>,
                          <span className="graph-operationbar-item" key="setting">
                            <Icon type="setting" onClick={() => {
                              this.graphConfigForm.showModal('update', <FormattedMessage id="graph.save" />, o);
                            }} />
                          </span>,
                          <span className="graph-operationbar-item" key="close">
                            <Icon type="close-circle-o" onClick={() => {
                              this.props.onChange('delete', o.id);
                            }} />
                          </span>,
                          <span className="graph-extra-item" key="more">
                            <Dropdown trigger={['click']} overlay={
                              <Menu>
                                <Menu.Item>
                                  <a onClick={() => { this.handleSubscribeGraph(o); }}><FormattedMessage id="graph.subscribe" /></a>
                                </Menu.Item>
                                <Menu.Item>
                                  <a onClick={() => { this.handleShareGraph(o); }}><FormattedMessage id="graph.share" /></a>
                                </Menu.Item>
                              </Menu>
                            }>
                              <span>
                                <Icon type="bars" />
                              </span>
                            </Dropdown>
                          </span>,
                        ];
                      }}
                    />
                  </div>
                </Col>
              );
            })
          }
          <Col span={24}>
            <div
              className={`${prefixCls}-graph ${prefixCls}-graph-add`}
              onClick={() => {
                this.graphConfigForm.showModal('push', <FormattedMessage id="graph.view" />);
              }}
              style={{ height: 350, cursor: 'pointer' }}
            >
              <div style={{ textAlign: 'center', width: '100%' }}>
                <Icon type="plus" /> <FormattedMessage id="graph.view" />
              </div>
            </div>
          </Col>
        </Row>
        <GraphConfig
          ref={(ref) => { this.graphConfigForm = ref; }}
          onChange={this.props.onGraphConfigSubmit}
        />
      </div>
    );
  }
}
