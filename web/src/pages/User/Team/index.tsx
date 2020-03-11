import React, { Component } from 'react';
import { Row, Col, Input, Divider, Popconfirm, Button, message } from 'antd';
import _ from 'lodash';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import FetchTable from '@cpts/FetchTable';
import { Team } from '@interface';
import PutTeam from './PutTeam';
import AddTeam from './AddTeam';

interface State {
  searchValue: string,
}

class UserTeam extends Component<null, State> {
  fetchtable: any;

  state = {} as State;

  handleAddBtnClick = () => {
    AddTeam({
      onOk: () => {
        this.fetchtable.reload();
      },
    });
  }

  handlePutBtnClick = (record: Team) => {
    PutTeam({
      data: {
        ...record,
        admins: _.map(record.admin_objs, n => n.id),
        members: _.map(record.member_objs, n => n.id),
      },
      onOk: () => {
        this.fetchtable.reload();
      },
    });
  }

  handleDelBtnClick = (id: number) => {
    request(`${api.team}/${id}`, {
      method: 'DELETE',
    }).then(() => {
      this.fetchtable.reload();
      message.success('团队删除成功！');
    });
  }

  render() {
    return (
      <div>
        <Row className="mb10">
          <Col span={8}>
            <Input.Search
              style={{ width: 200 }}
              onSearch={(val) => {
                this.setState({ searchValue: val });
              }}
            />
          </Col>
          <Col span={16} className="textAlignRight">
            <Button onClick={this.handleAddBtnClick} icon="plus">新建团队</Button>
          </Col>
        </Row>
        <FetchTable
          ref={(ref) => { this.fetchtable = ref; }}
          backendPagingEnabled={true}
          url={api.team}
          query={{ query: this.state.searchValue }}
          tableProps={{
            columns: [
              {
                title: '英文标识',
                dataIndex: 'ident',
                width: 130,
              }, {
                title: '中文名称',
                dataIndex: 'name',
                width: 130,
              }, {
                title: '管理员',
                dataIndex: 'admin_objs',
                render(text) {
                  const users = _.map(text, item => item.username);
                  return _.join(users, ', ');
                },
              }, {
                title: '普通成员',
                dataIndex: 'member_objs',
                render(text) {
                  const users = _.map(text, item => item.username);
                  return _.join(users, ', ');
                },
              }, {
                title: '操作',
                width: 100,
                render: (text, record) => {
                  return (
                    <span>
                      <a onClick={() => { this.handlePutBtnClick(record); }}>编辑</a>
                      <Divider type="vertical" />
                      <Popconfirm title="确认要删除这个团队吗？" onConfirm={() => { this.handleDelBtnClick(record.id); }}>
                        <a>删除</a>
                      </Popconfirm>
                    </span>
                  );
                },
              },
            ],
          }}
        />
      </div>
    );
  }
}
export default CreateIncludeNsTree(UserTeam);
