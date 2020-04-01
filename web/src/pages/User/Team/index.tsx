import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
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

class UserTeam extends Component<WrappedComponentProps, State> {
  fetchtable: any;

  state = {} as State;

  handleAddBtnClick = () => {
    AddTeam({
      title: <FormattedMessage id="table.create" />,
      language: this.props.intl.locale,
      onOk: () => {
        this.fetchtable.reload();
      },
    });
  }

  handlePutBtnClick = (record: Team) => {
    PutTeam({
      title: <FormattedMessage id="table.modify" />,
      language: this.props.intl.locale,
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
      message.success(this.props.intl.formatMessage({ id: 'msg.delete.success' }));
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
            <Button onClick={this.handleAddBtnClick} icon="plus">
              <FormattedMessage id="table.create" />
            </Button>
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
                title: <FormattedMessage id="team.ident" />,
                dataIndex: 'ident',
                width: 130,
              }, {
                title: <FormattedMessage id="team.name" />,
                dataIndex: 'name',
                width: 130,
              }, {
                title: <FormattedMessage id="team.admins" />,
                dataIndex: 'admin_objs',
                render(text) {
                  const users = _.map(text, item => item.username);
                  return _.join(users, ', ');
                },
              }, {
                title: <FormattedMessage id="team.members" />,
                dataIndex: 'member_objs',
                render(text) {
                  const users = _.map(text, item => item.username);
                  return _.join(users, ', ');
                },
              }, {
                title: <FormattedMessage id="table.operations" />,
                width: this.props.intl.locale === 'zh' ? 100 : 150,
                render: (_text, record) => {
                  return (
                    <span>
                      <a onClick={() => { this.handlePutBtnClick(record); }}><FormattedMessage id="table.modify" /></a>
                      <Divider type="vertical" />
                      <Popconfirm title={<FormattedMessage id="table.delete.sure" />} onConfirm={() => { this.handleDelBtnClick(record.id); }}>
                        <a><FormattedMessage id="table.delete" /></a>
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
export default CreateIncludeNsTree(injectIntl(UserTeam));
