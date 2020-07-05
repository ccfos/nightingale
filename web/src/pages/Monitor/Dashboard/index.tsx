import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { RouteComponentProps } from 'react-router-dom';
import PropTypes from 'prop-types';
import update from 'react-addons-update';
import { Layout, Row, Col, Button } from 'antd';
import _ from 'lodash';
import moment from 'moment';
import queryString from 'query-string';
import { config as graphConfig, GlobalOperationbar, services } from '@cpts/Graph';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import MetricSelect from './MetricSelect';
import Graphs from './Graphs';
import { prefixCls, baseMetrics } from './config';
import HostSelect from './HostSelect';
import SubscribeModal from './SubscribeModal';
import { normalizeGraphData } from './utils';
import { GraphData, Hosts, UpdateType, GraphId } from './interface';
import { TreeNode } from '@interface';
import './style.less';

type Props = RouteComponentProps;
interface State {
  graphs: GraphData[], // 所有图表配置
  selectedTreeNode: TreeNode | undefined, // 已选择的完整的节点信息
  metricsLoading: boolean,
  metrics: string[],
  hostsLoading: boolean,
  hosts: Hosts,
  selectedHosts: Hosts,
  globalOptions: {
    now: string,
    start: string,
    end: string,
    comparison: number[],
  },
}

const { Content } = Layout;

class MonitorDashboard extends Component<Props & WrappedComponentProps, State> {
  metricSelect: any;
  static contextTypes = {
    nsTreeVisibleChange: PropTypes.func,
    getSelectedNode: PropTypes.func,
    getNodes: PropTypes.func,
    habitsId: PropTypes.string,
  };

  allHostsMode = false;
  onceLoad = false;
  sidebarWidth = 200;

  constructor(props: Props & WrappedComponentProps) {
    super(props);
    const now = moment();
    this.state = {
      graphs: [],
      selectedTreeNode: undefined,
      metricsLoading: false,
      metrics: [],
      hostsLoading: false,
      hosts: [],
      selectedHosts: [],
      globalOptions: {
        now: now.clone().format('x'),
        start: now.clone().subtract(3600000, 'ms').format('x'),
        end: now.clone().format('x'),
        comparison: [],
      },
    };
  }

  componentWillReceiveProps = async (nextProps: Props) => {
    const { getSelectedNode, nsTreeVisibleChange } = this.context;
    const nextQuery = queryString.parse(_.get(nextProps, 'location.search'));

    if (nextQuery.mode === 'allHosts') {
      const selectedHosts = nextQuery.selectedHosts ? _.split(nextQuery.selectedHosts, ',') : [];
      if (!this.allHostsMode) {
        this.allHostsMode = true;
        nsTreeVisibleChange(false);
        const hosts = await this.fetchHosts();
        const metrics = await this.fetchMetrics(selectedHosts);
        this.setState({
          selectedHosts,
          selectedTreeNode: undefined,
          hosts,
          metrics,
        }, () => {
          if (!this.onceLoad) {
            this.processBaseMetrics();
            this.onceLoad = true;
          }
        });
      }
    } else {
      const selectedTreeNode = getSelectedNode();
      if (this.allHostsMode) {
        nsTreeVisibleChange(true);
        this.allHostsMode = false;
      }
      if (!_.isEqual(selectedTreeNode, this.state.selectedTreeNode)) {
        this.setState({ selectedTreeNode, graphs: [] });
        const hosts = await this.fetchHosts(_.get(selectedTreeNode, 'id'));
        this.setState({ hosts, selectedHosts: hosts });
        const metrics = await this.fetchMetrics(hosts);
        this.setState({ metrics });
      }
    }
  }

  async fetchHosts(nid?: number) {
    let hosts = [];
    try {
      this.setState({ hostsLoading: true });
      if (nid === undefined) {
        const res = await request(`${api.endpoint}?limit=1000`);
        hosts = _.map(res.list, 'ident');
      } else {
        hosts = await services.fetchEndPoints(nid);
      }
      this.setState({ hostsLoading: false });
    } catch (e) {
      console.log(e);
    }
    return hosts;
  }

  async fetchMetrics(selectedHosts: Hosts, hosts: Hosts = []) {
    let metrics = [];
    if (!_.isEmpty(selectedHosts)) {
      try {
        this.setState({ metricsLoading: true });
        metrics = await services.fetchMetrics(selectedHosts, hosts);
      } catch (e) {
        console.log(e);
      }
      this.setState({ metricsLoading: false });
    }
    return metrics;
  }

  async processBaseMetrics() {
    const { getSelectedNode } = this.context;
    const { selectedHosts, hosts } = this.state;
    const selectedTreeNode = getSelectedNode();
    const nid = _.get(selectedTreeNode, 'id');
    const now = moment();
    const newGraphs = [];

    for (let i = 0; i < baseMetrics.length; i++) {
      const tagkv = await services.fetchTagkv(selectedHosts, baseMetrics[i], hosts);
      const selectedTagkv = _.cloneDeep(tagkv);
      const endpointTagkv = _.find(selectedTagkv, { tagk: 'endpoint' });
      endpointTagkv.tagv = selectedHosts;

      newGraphs.push({
        id: Number(_.uniqueId()),
        now: now.clone().format('x'),
        start: now.clone().subtract(3600000, 'ms').format('x'),
        end: now.clone().format('x'),
        metrics: [{
          selectedNid: nid,
          selectedEndpoint: selectedHosts,
          endpoints: hosts,
          selectedMetric: baseMetrics[i],
          selectedTagkv,
          tagkv,
          aggrFunc: undefined,
          counterList: [],
        }],
      });

      this.setState({ graphs: newGraphs });
    }
  }

  handleGraphConfigSubmit = (type: UpdateType, data: GraphData, id?: GraphId) => {
    const { graphs } = this.state;
    const graphsClone = _.cloneDeep(graphs);
    const ldata = _.cloneDeep(data) || {};

    if (type === 'push') {
      // eslint-disable-next-line react/no-access-state-in-setstate
      this.setState(update(this.state, {
        graphs: {
          $push: [{
            ...graphConfig.graphDefaultConfig,
            id: Number(_.uniqueId()),
            ...ldata,
          }],
        },
      }));
    } else if (type === 'unshift') {
      this.setState({
        graphs: update(graphsClone, {
          $unshift: [{
            ...graphConfig.graphDefaultConfig,
            id: Number(_.uniqueId()),
            ...ldata,
          }],
        }),
      });
    } else if (type === 'update' && id) {
      this.handleUpdateGraph('update', id, {
        ...ldata,
      });
    }
  }

  handleUpdateGraph = (type: UpdateType, id: GraphId, updateConf?: GraphData, cbk?: () => void) => {
    const { graphs } = this.state;
    const index = _.findIndex(graphs, { id });
    if (type === 'allUpdate') {
      this.setState({
        graphs: updateConf,
      });
    } else if (type === 'update') {
      const currentConf = _.find(graphs, { id });
      // eslint-disable-next-line react/no-access-state-in-setstate
      this.setState(update(this.state, {
        graphs: {
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
    } else if (type === 'delete') {
      // eslint-disable-next-line react/no-access-state-in-setstate
      this.setState(update(this.state, {
        graphs: {
          $splice: [
            [index, 1],
          ],
        },
      }));
    }
  }

  handleBatchUpdateGraphs = (updateConf: GraphData) => {
    const { graphs } = this.state;
    const newPureGraphConfigs = _.map(graphs, (item) => {
      return {
        ...item,
        ...updateConf,
      };
    });

    this.setState({
      graphs: [...newPureGraphConfigs],
    });
  }

  handleSubscribeGraphs = () => {
    const configsList = _.map(this.state.graphs, (item) => {
      const data = normalizeGraphData(item);
      return JSON.stringify(data);
    });
    SubscribeModal({
      title: <FormattedMessage id="graph.subscribe" />,
      language: this.props.intl.locale,
      configsList,
    });
  }

  handleShareGraphs = () => {
    const configsList = _.map(this.state.graphs, (item) => {
      const data = normalizeGraphData(item);
      return {
        configs: JSON.stringify(data),
      };
    });
    request(api.tmpchart, {
      method: 'POST',
      body: JSON.stringify(configsList),
    }).then((res) => {
      window.open(`/#/monitor/tmpchart?ids=${_.join(res, ',')}`, '_blank');
    });
  }

  handleRemoveGraphs = () => {
    this.setState({ graphs: [] });
  }

  render() {
    const {
      selectedTreeNode,
      hostsLoading,
      hosts,
      selectedHosts,
      metricsLoading,
      metrics,
      graphs,
      globalOptions,
    } = this.state;
    if (!this.allHostsMode && !selectedTreeNode) {
      return (
        <div>
          <FormattedMessage id="please.select.node" />
        </div>
      );
    }
    return (
      <div className={prefixCls}>
        <Layout style={{ height: '100%', position: 'relative' }}>
          <Content>
            <Row gutter={10}>
              <Col span={12}>
                <HostSelect
                  graphConfigs={graphs}
                  loading={hostsLoading}
                  hosts={hosts}
                  selectedHosts={selectedHosts}
                  onSelectedHostsChange={async (newHosts, newSelectedHosts) => {
                    const newMetrics = await this.fetchMetrics(newSelectedHosts, hosts);
                    this.setState({ hosts: newHosts, selectedHosts: newSelectedHosts, metrics: newMetrics });
                  }}
                  updateGraph={(newGraphs) => {
                    this.setState({ graphs: newGraphs });
                  }}
                />
              </Col>
              <Col span={12}>
                <MetricSelect
                  nid={_.get(selectedTreeNode, 'id')}
                  loading={metricsLoading}
                  hosts={hosts}
                  selectedHosts={selectedHosts}
                  metrics={metrics}
                  graphs={graphs}
                  onSelect={(data) => {
                    this.handleGraphConfigSubmit('unshift', data);
                  }}
                />
              </Col>
            </Row>
            <Row style={{ padding: '10px 0' }}>
              <Col span={16}>
                <GlobalOperationbar
                  {...globalOptions}
                  onChange={(obj) => {
                    this.setState({
                      globalOptions: {
                        ...this.state.globalOptions,
                        ...obj,
                      } as any,
                    }, () => {
                      this.handleBatchUpdateGraphs(obj);
                    });
                  }}
                />
              </Col>
              <Col span={8} style={{ textAlign: 'right' }}>
                <Button
                  onClick={this.handleSubscribeGraphs}
                  disabled={!graphs.length}
                  style={{ background: '#fff', marginRight: 8 }}
                >
                  <FormattedMessage id="graph.subscribe" />
                </Button>
                <Button
                  onClick={this.handleShareGraphs}
                  disabled={!graphs.length}
                  style={{ background: '#fff', marginRight: 8 }}
                >
                  <FormattedMessage id="graph.share" />
                </Button>
                <Button
                  onClick={this.handleRemoveGraphs}
                  disabled={!graphs.length}
                  style={{ background: '#fff' }}
                >
                  <FormattedMessage id="graph.clear" />
                </Button>
              </Col>
            </Row>
            <Graphs
              value={graphs}
              onChange={this.handleUpdateGraph}
              onGraphConfigSubmit={this.handleGraphConfigSubmit}
            />
          </Content>
        </Layout>
      </div>
    );
  }
}

export default CreateIncludeNsTree(injectIntl(MonitorDashboard), { visible: true });
