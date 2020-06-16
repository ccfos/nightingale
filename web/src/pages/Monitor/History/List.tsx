import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Link } from 'react-router-dom';
import { Row, Col, Select, Input, DatePicker, Tag, Divider, message, Popconfirm, Badge, Button } from 'antd';
import { ColumnProps } from 'antd/lib/table';
import moment from 'moment';
import _ from 'lodash';
import FetchTable from '@cpts/FetchTable';
import request from '@common/request';
import api from '@common/api';
import { prefixCls, timeOptions, priorityOptions, eventTypeOptions } from '../config';
import { DataItem } from './interface';

interface Props {
  activeKey: string,
  type: string,
  nodepath: string,
  nid: number,
}

interface State {
  searchValue: string,
  data: any[],
  loading: boolean,
  customTime: boolean,
  stime: number,
  etime: number,
  priorities: undefined | string,
  type: undefined | string,
  nodepath: string,
}

const nPrefixCls = `${prefixCls}-history`;
const { Option } = Select;
const { Search } = Input;

class index extends Component<Props & WrappedComponentProps, State> {
  static defaultProps = {
    nodepath: undefined,
    nid: undefined,
  };
  fetchTable: any;
  otherParamsKey: string[];
  constructor(props: Props & WrappedComponentProps) {
    super(props);
    const now = moment();
    if (props.type === 'alert') {
      this.otherParamsKey = ['stime', 'etime', 'priorities', 'nodepath'];
    } else {
      this.otherParamsKey = ['stime', 'etime', 'priorities', 'nodepath', 'type'];
    }
    this.state = {
      searchValue: '',
      data: [],
      loading: false,
      customTime: false,
      stime: now.clone().subtract(2, 'hours').unix(),
      etime: now.clone().unix(),
      priorities: undefined,
      type: undefined,
      nodepath: props.nodepath,
    };
  }

  componentWillReceiveProps = (nextProps: Props) => {
    if (
      nextProps.nodepath &&
      (
        nextProps.nodepath !== this.props.nodepath ||
        nextProps.activeKey === nextProps.type
      )
    ) {
      this.updateTime(() => {
        this.setState({
          nodepath: nextProps.nodepath,
        });
      });
    }
  }

  getFetchDataUrl() {
    const { type, nodepath } = this.props;
    if (!nodepath) return;
    if (type === 'alert') {
      return `${api.event}/cur`;
    }
    return `${api.event}/his`;
  }

  getQuery() {
    const { nodepath } = this.props;
    const { stime, etime, priorities, type, searchValue } = this.state;

    return { stime, etime, priorities, nodepath, type, query: searchValue };
  }

  updateTime = (cbk?: () => void) => {
    const now = moment();
    const duration = this.state.etime - this.state.stime;
    this.setState({
      stime: now.clone().unix() - duration,
      etime: now.clone().unix(),
    }, () => {
      if (cbk) cbk();
    });
  }

  handleDelete = (id: number) => {
    request(`${api.event}/cur/${id}`, {
      method: 'DELETE',
    }).then(() => {
      message.success('忽略报警成功！');
      this.fetchTable.reload();
    });
  }

  handleClaim = (id: number) => {
    request(`${this.getFetchDataUrl()}s/claim`, {
      method: 'POST',
      body: JSON.stringify({ id }),
    }).then(() => {
      message.success('认领报警成功！');
      this.fetchTable.reload();
    });
  }

  handleClaimAll = () => {
    request(`${this.getFetchDataUrl()}s/claim`, {
      method: 'POST',
      body: JSON.stringify({
        nodepath: this.props.nodepath,
      }),
    }).then(() => {
      message.success('一健认领报警成功！');
      this.fetchTable.reload();
    });
  }

  getColumns() {
    const columns: ColumnProps<DataItem>[] = [
      {
        title: <FormattedMessage id="event.table.time" />,
        dataIndex: 'etime',
        fixed: 'left',
        width: 100,
        render: (text) => {
          return moment.unix(text).format('YYYY-MM-DD HH:mm:ss');
        },
      }, {
        title: <FormattedMessage id="event.table.stra" />,
        dataIndex: 'sname',
        width: 100,
        fixed: 'left',
      }, {
        title: <FormattedMessage id="event.table.priority" />,
        dataIndex: 'priority',
        width: 50,
        render: (text) => {
          const priorityObj = _.find(priorityOptions, { value: text });
          return (
            <Tag color={_.get(priorityObj, 'color')}>
              {_.get(priorityObj, 'label')}
            </Tag>
          );
        },
      }, {
        title: 'endpoint',
        dataIndex: 'endpoint',
      }, {
        title: 'tags',
        dataIndex: 'tags',
      }, {
        title: <FormattedMessage id="event.table.notify" />,
        dataIndex: 'status',
        fixed: 'right',
        width: 70,
        render: (text) => {
          return _.join(text, ', ');
        },
      }, {
        title: <FormattedMessage id="table.operations" />,
        fixed: 'right',
        // width: this.props.type === 'alert' ? 100 : 90,
        width: this.props.intl.locale === 'zh' ? 100 : 130,
        render: (text, record) => {
          return (
            <span>
              <Link
                to={{
                  pathname: `/monitor/history/${this.props.type === 'alert' ? 'cur' : 'his'}/${record.id}`,
                }}
                target="_blank"
              >
                <FormattedMessage id="table.detail" />
              </Link>
              {
                this.props.type === 'alert' ?
                  <span>
                    <Divider type="vertical" />
                    <Popconfirm title={<FormattedMessage id="event.table.ignore.sure" />} onConfirm={() => this.handleDelete(record.id)}>
                      <a><FormattedMessage id="event.table.ignore" /></a>
                    </Popconfirm>
                    <Divider type="vertical" />
                    <Popconfirm title={<FormattedMessage id="event.table.claim.sure" />} onConfirm={() => this.handleClaim(record.id)}>
                      <a><FormattedMessage id="event.table.claim" /></a>
                    </Popconfirm>
                  </span> : null
              }
              <Divider type="vertical" />
              <Link
                to={{
                  pathname: '/monitor/silence/add',
                  search: `${this.props.type === 'alert' ? 'cur' : 'his'}=${record.id}&nid=${this.props.nid}`,
                }}
                target="_blank"
              >
                <FormattedMessage id="event.table.shield" />
              </Link>
            </span>
          );
        },
      },
    ];
    if (this.props.type === 'alert') {
      columns.splice(5, 0, {
        title: <FormattedMessage id="event.table.assignees" />,
        dataIndex: 'claimants',
        width: 50,
        fixed: 'right',
        render: (text) => {
          return _.join(text, ', ');
        },
      });
    }
    if (this.props.type === 'all') {
      columns.splice(3, 0, {
        title: <FormattedMessage id="event.table.status" />,
        dataIndex: 'event_type',
        width: 70,
        render: (text) => {
          const eventTypeObj = _.find(eventTypeOptions, { value: text }) || {} as any;
          return (
            <span style={{ color: eventTypeObj.color }}>
              <Badge status={eventTypeObj.status} />
              <FormattedMessage id={`event.table.status.${eventTypeObj.value}`} />
            </span>
          );
        },
      });
    }
    return columns;
  }

  render() {
    const { customTime, stime, etime, priorities, type } = this.state;
    const duration = customTime ? 'custom' : (etime - stime) / (60 * 60);

    return (
      <div className={nPrefixCls}>
        <div className={`${nPrefixCls}-operationbar`} style={{ marginBottom: 10 }}>
          <Row>
            <Col span={18}>
              <Select
                style={{ width: 100, marginRight: 8 }}
                value={duration}
                onChange={(val: string | number) => {
                  if (val !== 'custom') {
                    const now = moment();
                    const nStime = now.clone().subtract(val, 'hours').unix();
                    const nEtime = now.clone().unix();
                    this.setState({ customTime: false, stime: nStime, etime: nEtime });
                  } else {
                    this.setState({ customTime: true });
                  }
                }}
              >
                {
                  _.map(timeOptions, (option) => {
                    return <Option key={option.value} value={option.value}><FormattedMessage id={option.label} /></Option>;
                  })
                }
              </Select>
              {
                customTime ?
                  <span>
                    <DatePicker
                      style={{ marginRight: 8 }}
                      showTime
                      format="YYYY-MM-DD HH:mm:ss"
                      value={moment.unix(stime)}
                      placeholder="Start"
                      onChange={(val) => {
                        if (val) {
                          this.setState({ stime: val.unix() });
                        }
                      }}
                    />
                    <DatePicker
                      style={{ marginRight: 8 }}
                      showTime
                      format="YYYY-MM-DD HH:mm:ss"
                      value={moment.unix(etime)}
                      placeholder="End"
                      onChange={(val) => {
                        if (val) {
                          this.setState({ etime: val.unix() });
                        }
                      }}
                    />
                  </span> : null
              }
              {
                this.props.type === 'all' ?
                  <Select
                    style={{ minWidth: 90, marginRight: 8 }}
                    placeholder={this.props.intl.formatMessage({ id: 'event.table.status' })}
                    allowClear
                    value={type}
                    onChange={(value: string) => {
                      this.updateTime(() => {
                        this.setState({ type: value });
                      });
                    }}
                  >
                    {
                      _.map(eventTypeOptions, (option) => {
                        return <Option key={option.value} value={option.value}><FormattedMessage id={`event.table.status.${option.value}`} /></Option>;
                      })
                    }
                  </Select> : null
              }
              <Select
                style={{ minWidth: 90, marginRight: 8 }}
                placeholder={this.props.intl.formatMessage({ id: 'event.table.priority' })}
                allowClear
                mode="multiple"
                value={priorities ? _.map(_.split(priorities, ','), _.toNumber) : []}
                onChange={(value: number) => {
                  this.updateTime(() => {
                    this.setState({ priorities: !_.isEmpty(value) ? _.join(value, ',') : undefined });
                  });
                }}
              >
                {
                  _.map(priorityOptions, (option) => {
                    return <Option key={option.value} value={option.value}>{option.label}</Option>;
                  })
                }
              </Select>
              <Search
                placeholder="Search"
                style={{ width: 200 }}
                onSearch={(value) => {
                  this.setState({ searchValue: value });
                }}
              />
            </Col>
            <Col span={6} style={{ textAlign: 'right' }}>
              {
                this.props.type === 'alert' ?
                  <Popconfirm title={<FormattedMessage id="event.table.claim.all.sure" />} onConfirm={() => this.handleClaimAll()}>
                    <Button><FormattedMessage id="event.table.claim.all" /></Button>
                  </Popconfirm> : null
              }
            </Col>
          </Row>
        </div>
        <div className="alarm-strategy-content">
          <FetchTable
            ref={(ref) => this.fetchTable = ref }
            url={this.getFetchDataUrl()}
            query={this.getQuery()}
            tableProps={{
              columns: this.getColumns(),
              scroll: { x: 900 },
            }}
          />
        </div>
      </div>
    );
  }
}

export default injectIntl(index);
