import React, { Component } from 'react';
import ReactDOM from 'react-dom';
import { Modal, Form, Select, Radio } from 'antd';
import _ from 'lodash';

interface Props {
  title: string,
  visible: boolean,
  data: any,
  tags: any,
  onOk: (values: any) => void | boolean,
  destroy: () => void,
}

interface State {
  data: any;
}

const FormItem = Form.Item;
const { Option } = Select;
const RadioGroup = Radio.Group;
const formItemLayout = {
  labelCol: { span: 4 },
  wrapperCol: { span: 16 },
};
const toptMap = {
  '=': '包含',
  '!=': '排除',
};

class FilterFormModal extends Component<Props, State> {
  static defaultProps = {
    title: '',
    visible: false,
    data: {},
    tags: {},
    onOk: _.noop,
    destroy: _.noop,
  };

  formId = _.uniqueId('tagFilterConditionForm');
  constructor(props: Props) {
    super(props);
    this.state = {
      data: {
        topt: '=',
        ...props.data,
      },
    } as State;
  }

  componentWillReceiveProps(nextProps: Props) {
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
    let tvalOptions: any[] = [];

    if (!_.isEmpty(tags) && data.tkey) {
      const tval = _.filter(tags[data.tkey], (item, i: number) => i < 500);
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

  handleFieldChange = (field: string, val: any) => {
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
            label="Tag 名称"
            validateStatus={_.isEmpty(data.tkey) ? 'error' : ''}
            help={_.isEmpty(data.tkey) && '不能为空'}
          >
            <Select
              mode="combobox"
              notFoundContent=""
              placeholder="支持自定义，非叶子节点或者'与'条件时没有自动补全功能"
              defaultActiveFirstOption={false}
              // getPopupContainer={() => document.getElementById(this.formId)}
              value={data.tkey}
              onChange={(val: any) => this.handleFieldChange('tkey', val)}
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
                _.map(toptMap, (val, key) => <Radio key={key} value={key}>{val}</Radio>)
              }
            </RadioGroup>
          </FormItem>
          <FormItem
            {...formItemLayout}
            label="Tag 取值"
            validateStatus={_.isEmpty(data.tval) ? 'error' : ''}
            help={_.isEmpty(data.tval) && '不能为空'}
          >
            <Select
              mode="tags"
              showSearch
              notFoundContent=""
              placeholder="支持自定义，必须完全匹配不支持正则"
              // getPopupContainer={() => document.getElementById(this.formId)}
              value={data.tval}
              onChange={(val: any) => this.handleFieldChange('tval', val)}
            >
              {tvalOptions}
            </Select>
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default function filterFormModal(config: any) {
  const div = document.createElement('div');
  document.body.appendChild(div);

  function destroy() {
    const unmountResult = ReactDOM.unmountComponentAtNode(div);
    if (unmountResult && div.parentNode) {
      div.parentNode.removeChild(div);
    }
  }

  function render(props: any) {
    ReactDOM.render(<FilterFormModal {...props} />, div);
  }

  render({ ...config, visible: true, destroy });

  return {
    destroy,
  };
}
