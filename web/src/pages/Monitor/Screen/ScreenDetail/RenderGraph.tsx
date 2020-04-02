import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import { Popconfirm, Menu, Col } from 'antd';
import { SortableElement } from 'react-sortable-hoc';
import _ from 'lodash';
import Graph from '@cpts/Graph';
import request from '@common/request';
import api from '@common/api';
import { normalizeGraphData } from '../../Dashboard/utils';

class RenderGraph extends Component<any> {
  shouldComponentUpdate = (nextProps: any) => {
    return !_.isEqual(nextProps.data, this.props.data) ||
    !_.isEqual(nextProps.subclassData, this.props.subclassData) ||
    nextProps.index !== this.props.index ||
    nextProps.colNum !== this.props.colNum;
  }

  handleShareGraph = (graphData: any) => {
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

  handleCloneGraph = (configs: any) => {
    this.props.onCloneGraph(configs);
  }

  render() {
    const { data, originTreeData, subclassData, colNum } = this.props;

    return (
      <Col span={24 / colNum}>
        <Graph
          useDragHandle
          ref={(ref) => { this.props.graphsInstance[data.id] = ref; }}
          height={180}
          graphConfigInnerVisible={false}
          treeData={originTreeData}
          data={{
            ...data.configs,
            id: data.id,
          }}
          onOpenGraphConfig={(graphOptions: any) => {
            this.props.graphConfigForm.showModal('update', '保存', {
              ...graphOptions,
              subclassId: data.subclass_id,
              isScreen: true,
              subclassOptions: subclassData,
            });
          }}
          extraMoreList={[
            <Menu.Item key="share">
              <a onClick={() => { this.handleShareGraph(data.configs); }}><FormattedMessage id="screen.graph.extraMoreList.share" /></a>
            </Menu.Item>,
            <Menu.Item key="clone">
            <a onClick={() => { this.handleCloneGraph(data.configs); }}><FormattedMessage id="screen.graph.extraMoreList.clone" /></a>
          </Menu.Item>,
            <Menu.Item key="del">
              <Popconfirm title={<FormattedMessage id="screen.graph.extraMoreList.delete.sure" />} onConfirm={() => { this.props.onDelChart(data.id); }}>
                <a><FormattedMessage id="screen.graph.extraMoreList.delete" /></a>
              </Popconfirm>
            </Menu.Item>,
          ]}
        />
      </Col>
    );
  }
}

export default SortableElement(RenderGraph);
