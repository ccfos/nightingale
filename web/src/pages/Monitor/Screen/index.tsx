import React, { Component } from 'react';
import { FormattedMessage, WrappedComponentProps, injectIntl } from 'react-intl';
import PropTypes from 'prop-types';
import { Link } from 'react-router-dom';
import { Button, Input, Divider, Popconfirm, Table, message } from 'antd';
import { DragDropContext, DragSource, DropTarget } from 'react-dnd';
import HTML5Backend from 'react-dnd-html5-backend';
import _ from 'lodash';
import update from 'immutability-helper';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import { appname } from '@common/config';
import AddModal from './AddModal';
import ModifyModal from './ModifyModal';
import './style.less';

let dragingIndex = -1;

class BodyRow extends Component<any> {
  render() {
    const {
      isOver,
      connectDragSource,
      connectDropTarget,
      moveRow,
      ...restProps
    } = this.props;
    const style = { ...restProps.style, cursor: 'move' };

    let { className } = restProps;
    if (isOver) {
      if (restProps.index > dragingIndex) {
        className += ' drop-over-downward';
      }
      if (restProps.index < dragingIndex) {
        className += ' drop-over-upward';
      }
    }

    return connectDragSource(
      connectDropTarget(
        <tr
          {...restProps}
          className={className}
          style={style}
        />,
      ),
    );
  }
}

const rowSource = {
  beginDrag(props: any) {
    dragingIndex = props.index;
    return {
      index: props.index,
    };
  },
};

const rowTarget = {
  drop(props: any, monitor: any) {
    const dragIndex = monitor.getItem().index;
    const hoverIndex = props.index;

    if (dragIndex === hoverIndex) {
      return;
    }

    props.moveRow(dragIndex, hoverIndex);
    monitor.getItem().index = hoverIndex;
  },
};

const DragableBodyRow = DropTarget(
  'row',
  rowTarget,
  (connect, monitor) => ({
    connectDropTarget: connect.dropTarget(),
    isOver: monitor.isOver(),
  }),
)(
  DragSource(
    'row',
    rowSource,
    connect => ({
      connectDragSource: connect.dragSource(),
    }),
  )(BodyRow),
);

class Screen extends Component<WrappedComponentProps> {
  static contextTypes = {
    getSelectedNode: PropTypes.func,
  };
  selectedNodeId: number | undefined = undefined;
  state = {
    loading: false,
    data: [],
    search: '',
    selectedNode: undefined as any,
  };

  componentDidMount = () => {
    this.fetchData();
  }

  componentWillMount = () => {
    const { getSelectedNode } = this.context;
    this.selectedNodeId = getSelectedNode('id');
  }

  componentWillReceiveProps = () => {
    const { getSelectedNode } = this.context;
    const selectedNode = getSelectedNode();

    if (!_.isEqual(selectedNode, this.state.selectedNode)) {
      this.setState({
        selectedNode,
      }, () => {
        this.selectedNodeId = getSelectedNode('id');
        this.fetchData();
      });
    }
  }

  fetchData() {
    if (this.selectedNodeId) {
      this.setState({ loading: true });
      request(`${api.node}/${this.selectedNodeId}/screen`).then((res) => {
        this.setState({ data: _.sortBy(res, 'weight') });
      }).finally(() => {
        this.setState({ loading: false });
      });
    }
  }

  handleAdd = () => {
    AddModal({
      language: this.props.intl.locale,
      title: this.props.intl.formatMessage({ id: 'table.create' }),
      onOk: (values: any) => {
        request(`${api.node}/${this.selectedNodeId}/screen`, {
          method: 'POST',
          body: JSON.stringify({
            ...values,
            weight: this.state.data.length,
          }),
        }).then(() => {
          message.success('新增大盘成功！');
          this.fetchData();
        });
      },
    });
  }

  handleModify = (record: any) => {
    ModifyModal({
      language: this.props.intl.locale,
      name: record.name,
      title: this.props.intl.formatMessage({ id: 'table.modify' }),
      onOk: (values: any) => {
        request(`${api.screen}/${record.id}`, {
          method: 'PUT',
          body: JSON.stringify({
            ...values,
            node_id: record.node_id,
          }),
        }).then(() => {
          message.success(this.props.intl.formatMessage({ id: 'msg.modify.success' }));
          this.fetchData();
        });
      },
    });
  }

  handleDel = (id: number) => {
    request(`${api.screen}/${id}`, {
      method: 'DELETE',
    }).then(() => {
      message.success(this.props.intl.formatMessage({ id: 'msg.delete.success' }));
      this.fetchData();
    });
  }

  moveRow = (dragIndex: number, hoverIndex: number) => {
    const { data } = this.state;
    const dragRow = data[dragIndex];

    this.setState(
      // eslint-disable-next-line react/no-access-state-in-setstate
      update(this.state, {
        data: {
          $splice: [[dragIndex, 1], [hoverIndex, 0, dragRow]],
        },
      }),
      () => {
        const reqBody = _.map(this.state.data, (item: any, i) => {
          return {
            id: item.id,
            weight: i,
          };
        });
        request(`${api.chart}s/weights`, {
          method: 'PUT',
          body: JSON.stringify(reqBody),
        }).then(() => {
          message.success(this.props.intl.formatMessage({ id: 'msg.sort.success' }));
        });
      },
    );
  }

  filterData() {
    const { data, search } = this.state;
    if (search) {
      return _.filter(data, (item: any) => {
        return item.name.indexOf(search) > -1;
      });
    }
    return data;
  }

  render() {
    const { search } = this.state;
    const prefixCls = `${appname}-monitor-screen`;
    const tableData = this.filterData();
    return (
      <div className={prefixCls}>
        <div className="mb10">
          <Button className="mr10" onClick={this.handleAdd}><FormattedMessage id="screen.create" /></Button>
          <Input
            style={{ width: 200 }}
            placeholder="Search"
            value={search}
            onChange={(e) => {
              this.setState({ search: e.target.value });
            }}
          />
        </div>
        <Table
          rowKey="id"
          dataSource={tableData}
          pagination={false}
          components={{
            body: {
              row: DragableBodyRow,
            },
          }}
          onRow={(record, index) => ({
            index,
            moveRow: this.moveRow,
          })}
          columns={[
            {
              title: <FormattedMessage id="table.name" />,
              dataIndex: 'name',
              render: (text, record) => {
                return <Link to={{ pathname: `/monitor/screen/${record.id}` }}>{text}</Link>;
              },
            }, {
              title: <FormattedMessage id="table.creator" />,
              width: 200,
              dataIndex: 'last_updator',
            }, {
              title: <FormattedMessage id="table.operations" />,
              width: 200,
              render: (text, record: any) => {
                return (
                  <span>
                    <a onClick={() => this.handleModify(record)}><FormattedMessage id="table.modify" /></a>
                    <Divider type="vertical" />
                    <Popconfirm title={<FormattedMessage id="table.delete.sure" />} onConfirm={() => this.handleDel(record.id)}>
                      <a><FormattedMessage id="table.delete" /></a>
                    </Popconfirm>
                  </span>
                );
              },
            },
          ]}
        />
      </div>
    );
  }
}

export default CreateIncludeNsTree(DragDropContext(HTML5Backend)(injectIntl(Screen)), { visible: true });
