import React, { Component } from 'react';
import { Icon } from 'antd';
import update from 'immutability-helper';
import queryString from 'query-string';
import PubSub from 'pubsub-js';
import _ from 'lodash';
import Graph, { GraphConfig, Info } from '@cpts/Graph';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';

class Tmpchart extends Component<any> {
  graphConfigForm: any;
  state = {
    data: [],
  };
  graphs: any = {};

  componentDidMount = () => {
    this.fetchData(this.props);
    PubSub.subscribe('sider-collapse', () => {
      this.resizeGraphs();
    });
  }

  fetchData(props: any) {
    const search = _.get(props, 'location.search');

    if (search) {
      const query = queryString.parse(search);
      request(`${api.tmpchart}?ids=${query.ids}`).then((res) => {
        const data = _.map(res, (item) => {
          let { configs } = item;
          try {
            configs = JSON.parse(configs);
          } catch (e) {
            console.log(e);
          }
          if (!configs.id) {
            configs.id = (new Date()).getTime();
          }
          return configs;
        });
        this.setState({ data });
      });
    }
  }

  resizeGraphs = () => {
    _.each(this.graphs, (graph) => {
      if (graph) {
        graph.resize();
      }
    });
  }

  handleUpdateGraph = (type: string, id: number, updateConf: any, cbk?: () => void) => {
    const { data } = this.state;
    const index = _.findIndex(data, { id });
    if (type === 'allUpdate') {
      this.setState({
        data: updateConf,
      });
    } else if (type === 'update') {
      const currentConf = _.find(data, { id });
      // eslint-disable-next-line react/no-access-state-in-setstate
      this.setState(update(this.state, {
        data: {
          $splice: [
            [index, 1, {
              ...currentConf,
              ...updateConf,
            }],
          ],
        },
      }), () => {
        if (cbk) cbk();
      });
    }
  }

  handleGraphConfigChange = (type: string, data: any, id: number) => {
    if (type === 'update') {
      this.handleUpdateGraph('update', id, {
        ...data,
      });
    }
  }

  render() {
    const { data } = this.state;
    return (
      <div>
        {
          _.map(data, (item) => {
            const { id } = item;
            return (
              <div
                key={id}
                style={{ marginBottom: 10 }}
              >
                <Graph
                  ref={(ref) => { this.graphs[item.id] = ref; }}
                  data={{
                    id,
                    ...item,
                  }}
                  onChange={this.handleUpdateGraph}
                  extraRender={(graph) => {
                    return [
                      <span className="graph-operationbar-item" key="info" title="详情">
                        <Info
                          graphConfig={graph.getGraphConfig(graph.props.data)}
                          counterList={graph.counterList}
                        >
                          <Icon type="info-circle-o" />
                        </Info>
                      </span>,
                      <span className="graph-operationbar-item" key="setting" title="编辑">
                        <Icon type="setting" onClick={() => {
                          this.graphConfigForm.showModal('update', '保存', item);
                        }} />
                      </span>,
                    ];
                  }}
                />
              </div>
            );
          })
        }
        <GraphConfig
          ref={(ref) => { this.graphConfigForm = ref; }}
          onChange={this.handleGraphConfigChange}
        />
      </div>
    );
  }
}

export default CreateIncludeNsTree(Tmpchart, { visible: false });
