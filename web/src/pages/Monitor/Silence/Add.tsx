import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Button, Row, Col, message } from 'antd';
import _ from 'lodash';
import moment from 'moment';
import queryString from 'query-string';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import CustomForm from './CustomForm';
import { normalizReqData } from './utils';

class Add extends Component<WrappedComponentProps> {
  customForm: any;
  state = {
    nid: undefined,
    initialValues: {},
    submitLoading: false,
  };

  componentDidMount = () => {
    const search = _.get(this.props, 'location.search');
    const query = queryString.parse(search);

    if (query && (query.cur || query.his)) {
      const type = query.cur ? 'cur' : 'his';
      const id = query.cur || query.his;
      this.fetchHistoryData(type, id);
    }
    if (query && query.nid) {
      this.setState({ nid: _.toNumber(query.nid) });
    }
  }

  fetchHistoryData(type: string, id: number) {
    request(`${api.event}/${type}/${id}`).then((res) => {
      this.setState({
        initialValues: {
          metric: _.get(res, 'detail[0].metric'),
          endpoints: _.get(res, 'endpoint'),
          tags: res.tags,
        },
      });
    });
  }

  handleSubmit = () => {
    const { history } = this.props;
    this.customForm.validateFields((errors: any, data: any) => {
      if (!errors) {
        const reqData = normalizReqData(data) as any;
        reqData.nid = this.state.nid;

        this.setState({ submitLoading: true });
        request(api.maskconf, {
          method: 'POST',
          body: JSON.stringify(reqData),
        }).then(() => {
          message.success(this.props.intl.formatMessage({ id: 'msg.create.success' }));
          history.push({
            pathname: '/monitor/silence',
          });
        }).catch(() => {
          // message.error('新增屏蔽失败！');
        }).finally(() => {
          this.setState({ submitLoading: false });
        });
      }
    });
  }

  render() {
    const { submitLoading, initialValues } = this.state;
    const now = moment();

    return (
      <div>
        <CustomForm
          ref={(ref) => { this.customForm = ref; }}
          initialValues={{
            btime: now.clone().unix(),
            etime: now.clone().add(1, 'hours').unix(),
            cause: this.props.intl.formatMessage({ id: 'silence.cause.default' }),
            ...initialValues,
          }}
        />
        <Row>
          <Col offset={6}>
            <Button onClick={this.handleSubmit} loading={submitLoading} type="primary">
              <FormattedMessage id="form.submit" />
            </Button>
          </Col>
        </Row>
      </div>
    );
  }
}

export default CreateIncludeNsTree(injectIntl(Add));
