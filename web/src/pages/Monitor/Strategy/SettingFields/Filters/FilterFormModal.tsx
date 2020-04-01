import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Modal, Form, Select, Radio } from 'antd';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';
import ModalControl from '@cpts/ModalControl';

const FormItem = Form.Item;
const { Option } = Select;
const RadioGroup = Radio.Group;
const formItemLayout = {
  labelCol: { span: 4 },
  wrapperCol: { span: 16 },
};
const toptMap = {
  '=': 'stra.tag.include',
  '!=': 'stra.tag.exclude',
};

class FilterFormModal extends Component {
  static propTypes = {
    title: PropTypes.string,
    visible: PropTypes.bool,
    data: PropTypes.object,
    tags: PropTypes.object,
    onOk: PropTypes.func,
    destroy: PropTypes.func,
  };

  static defaultProps = {
    title: '',
    visible: false,
    data: {},
    tags: {},
    onOk: _.noop,
    destroy: _.noop,
  };

  constructor(props) {
    super(props);
    this.formId = _.uniqueId('tagFilterConditionForm');
    this.state = {
      data: {
        topt: '=',
        ...props.data,
      },
    };
  }

  componentWillReceiveProps(nextProps) {
    if (!_.isEqual(nextProps.data, this.props.data)) {
      this.setState({
        data: {
          topt: '=',
          ...nextProps.data,
        },
      });
    }
  }

  getTvalOptions() {
    const { tags } = this.props;
    const { data } = this.state;
    let tvalOptions = [];

    if (!_.isEmpty(tags) && data.tkey) {
      const tval = _.filter(tags[data.tkey], (item, i) => i < 500);
      tvalOptions = _.map(tval, item => <Option key={item} value={item}>{item}</Option>);
    }
    return tvalOptions;
  }

  handleOk = () => {
    const { data } = this.state;

    if (_.isEmpty(data.tkey) || _.isEmpty(data.tval)) return;

    const reuslt = this.props.onOk({ ...data });

    if (reuslt === undefined || reuslt === true) {
      this.props.destroy();
    }
  }

  handleCancel = () => {
    this.props.destroy();
  }

  handleFieldChange = (field, val) => {
    const { data } = this.state;
    this.setState({
      data: {
        ...data,
        [field]: val,
      },
    });
  }

  render() {
    const { title, visible, tags } = this.props;
    const { data } = this.state;
    const tvalOptions = this.getTvalOptions();

    return (
      <Modal
        width={600}
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <Form
          id={this.formId}
          style={{ width: 600 }}
        >
          <FormItem
            {...formItemLayout}
            label="Tag name"
            validateStatus={_.isEmpty(data.tkey) ? 'error' : ''}
            help={_.isEmpty(data.tkey) && 'is required'}
          >
            <Select
              mode="combobox"
              notFoundContent=""
              // placeholder="支持自定义，非叶子节点或者'与'条件时没有自动补全功能"
              defaultActiveFirstOption={false}
              // getPopupContainer={() => document.getElementById(this.formId)}
              value={data.tkey}
              onChange={val => this.handleFieldChange('tkey', val)}
            >
              {
                _.map(tags, (tval, tkey) => <Option key={tkey} value={tkey}>{tkey}</Option>)
              }
            </Select>
          </FormItem>
          <FormItem wrapperCol={{ span: 16, offset: 4 }}>
            <RadioGroup
              value={data.topt}
              onChange={e => this.handleFieldChange('topt', e.target.value)}
            >
              {
                _.map(toptMap, (val, key) => <Radio key={key} value={key}><FormattedMessage id={val} /></Radio>)
              }
            </RadioGroup>
          </FormItem>
          <FormItem
            {...formItemLayout}
            label="Tag value"
            validateStatus={_.isEmpty(data.tval) ? 'error' : ''}
            help={_.isEmpty(data.tval) && 'is required'}
          >
            <Select
              mode="tags"
              showSearch
              notFoundContent=""
              // placeholder="支持自定义，必须完全匹配不支持正则"
              // getPopupContainer={() => document.getElementById(this.formId)}
              value={data.tval}
              onChange={val => this.handleFieldChange('tval', val)}
            >
              {tvalOptions}
            </Select>
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(FilterFormModal);
