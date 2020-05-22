import React, { Component }from 'react';
import { FormattedMessage } from 'react-intl';
import { Form, Input, Radio, Select, Spin } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import { Team, UserProfile } from '@interface';
import request from '@common/request';
import api from '@common/api';

interface Props {
  initialValue: Team,
}

interface State {
  users: UserProfile[],
  value: string,
  fetching: boolean,
}

const FormItem = Form.Item;
const RadioGroup = Radio.Group;
const { Option } = Select;

class TeamForm extends Component<Props & FormProps, State> {
  static defaultProps: any = {
    initialValue: {} as any,
  };

  lastFetchId = 0;

  constructor(props: Props & FormProps) {
    super(props);
    this.fetchUser = _.debounce(this.fetchUser, 500);
  }

  state = {
    users: [],
    value: '',
    fetching: false,
  } as State;

  componentDidMount() {
    this.fetchUser();
  }

  fetchUser = (query = '') => {
    this.lastFetchId += 1;
    const fetchId = this.lastFetchId;
    this.setState({ users: [], fetching: true });
    request(`${api.user}?limit=1000&query=${query}`).then((res) => {
      if (fetchId !== this.lastFetchId) {
        // for fetch callback order
        return;
      }
      this.setState({ users: res.list, fetching: false });
    });
  };

  validateFields() {
    return this.props.form!.validateFields;
  }

  renderUserSelect() {
    const { users, fetching } = this.state;

    return (
      <Select
        mode="multiple"
        showSearch
        filterOption={false}
        notFoundContent={fetching ? <Spin size="small" /> : null}
        onSearch={this.fetchUser}
        onDropdownVisibleChange={(open) => {
          if (!open) {
            this.fetchUser();
          }
        }}
      >
        {
          _.map(users, (item) => {
            return <Option key={item.id} value={item.id}>{item.username}</Option>;
          })
        }
      </Select>
    );
  }

  render() {
    const { initialValue } = this.props;
    const { getFieldDecorator, getFieldValue } = this.props.form!;
    return (
      <Form layout="vertical">
        <FormItem label={<FormattedMessage id="team.ident" />} required>
          {getFieldDecorator('ident', {
            initialValue: initialValue.ident,
            rules: [{ required: true }],
          })(
            <Input />,
          )}
        </FormItem>
        <FormItem label={<FormattedMessage id="team.name" />} required>
          {getFieldDecorator('name', {
            initialValue: initialValue.name,
            rules: [{ required: true }],
          })(
            <Input />,
          )}
        </FormItem>
        <FormItem label={<FormattedMessage id="team.mgmt" />} required>
          {getFieldDecorator('mgmt', {
            initialValue: initialValue.mgmt || 0,
            rules: [{ required: true }],
          })(
            <RadioGroup>
              <Radio value={0}><FormattedMessage id="team.mgmt.member" /></Radio>
              <Radio value={1}><FormattedMessage id="team.mgmt.admin" /></Radio>
            </RadioGroup>,
          )}
        </FormItem>
        {
          getFieldValue('mgmt') === 1 ?
            <FormItem label={<FormattedMessage id="team.admins" />}>
              {getFieldDecorator('admins', {
                initialValue: initialValue.admins,
                rules: [{
                  required: getFieldValue('mgmt') === 1,
                  // message: '管理员管理制必须选择管理员!',
                }],
              })(
                this.renderUserSelect(),
              )}
            </FormItem> : null
        }
        <FormItem label={<FormattedMessage id="team.members" />}>
          {getFieldDecorator('members', {
            initialValue: initialValue.members,
          })(
            this.renderUserSelect(),
          )}
        </FormItem>
      </Form>
    );
  }
}

export default Form.create()(TeamForm);
