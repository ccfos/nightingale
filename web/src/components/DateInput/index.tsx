import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Input, Icon, Popover, Tooltip } from 'antd';
import Calendar from 'rc-calendar';
import moment from 'moment';
import _ from 'lodash';
import enUS from 'rc-calendar/lib/locale/en_US';
import 'rc-calendar/assets/index.css';
import './style.less';

function transformGregorianToMoment(format: string) {
  return _.chain(format).replace('yyyy', 'YYYY').replace('dd', 'DD').value();
}

export default class DateInput extends Component<any, any> {
  static propTypes = {
    size: PropTypes.string, // antd Input size
    format: PropTypes.string.isRequired, // gregorian format
    locale: PropTypes.object,
    style: PropTypes.object,
    value: PropTypes.string,
    onChange: PropTypes.func,
  };

  static defaultProps = {
    size: 'default',
    value: undefined,
    locale: {},
    style: {},
    onChange: _.noop,
  };

  constructor(props: any) {
    super(props);
    this.state = {
      tempValue: props.value,
      tempSelectedValue: props.value,
      invalid: false,
      popoverVisible: false,
      tooltipVisible: false,
    };
  }

  componentDidMount() {
    this.checkTempValue();
  }

  componentWillReceiveProps(nextProps: any) {
    if (nextProps.value !== this.props.value) {
      this.setState({
        tempValue: nextProps.value,
        tempSelectedValue: nextProps.value,
      });
    }
  }

  checkTempValue = () => {
    const { format } = this.props;
    const { tempValue } = this.state;
    const momentFormat = transformGregorianToMoment(format);
    const date = moment(tempValue, momentFormat, true);
    let invalid = false;

    if (date.format() === 'Invalid date') {
      invalid = true;
    }

    this.setState({ invalid });
  }

  handleBlur = () => {
    const { invalid, tempValue } = this.state;
    if (!invalid) {
      this.props.onChange(moment(tempValue).toDate());
    } else {
      this.setState({
        tempValue: this.props.value,
        tooltipVisible: false,
      });
    }
  }

  handleKeyUp = (e: any) => {
    const { invalid, tempValue } = this.state;
    if (e.keyCode === 13 && !invalid) {
      this.props.onChange(moment(tempValue).toDate());
    }
  }

  handleChange = (e: any) => {
    const { value } = e.target;
    this.setState({ tempValue: value }, () => {
      const invalid = this.checkTempValue();
      this.setState({ invalid, tooltipVisible: invalid });
    });
  }

  closePopover = () => {
    this.setState({
      popoverVisible: false,
      tempSelectedValue: this.props.value,
    });
  }

  render() {
    const {
      size, style, format, locale, onChange,
    } = this.props;
    const {
      tempValue, tempSelectedValue, popoverVisible, tooltipVisible,
    } = this.state;
    const momentFormat = transformGregorianToMoment(format);
    const selectedValue = tempSelectedValue ? moment(tempSelectedValue) : null;

    return (
      <span className="dateInput" style={{
        ...style,
        minWidth: 208,
        display: 'inline-block',
        verticalAlign: 'top',
      }}>
        <Popover
          visible={popoverVisible}
          trigger="click"
          placement="bottomLeft"
          overlayClassName="dateInput-popover"
          content={
            <Calendar
              className="dateInput-calendar"
              showOk
              format={momentFormat}
              locale={{
                ...enUS,
                ...locale,
              }}
              selectedValue={selectedValue}
              onOk={(mDate: any) => {
                onChange(mDate.toDate());
                this.closePopover();
              }}
              onClear={() => {
                this.closePopover();
              }}
              onSelect={(mDate: any) => {
                if (mDate && mDate.format() !== 'Invalid date') {
                  this.setState({ tempSelectedValue: mDate.format(momentFormat) });
                }
              }}
            />
          }
          onVisibleChange={() => {
            this.closePopover();
          }}
        >
          <Tooltip
            visible={tooltipVisible}
            title={
              <span>
                <Icon type="exclamation-circle-o" /> 请按照 {momentFormat} 格式填写
              </span>
            }
          >
            <Input
              size={size}
              value={tempValue}
              onBlur={this.handleBlur}
              onKeyUp={this.handleKeyUp}
              onChange={this.handleChange}
              placeholder={momentFormat}
              addonAfter={
                <Icon
                  title="时间选择"
                  type="calendar"
                  onClick={() => {
                    if (popoverVisible) {
                      this.closePopover();
                    } else {
                      this.setState({ popoverVisible: true });
                    }
                  }}
                />
              }
            />
          </Tooltip>
        </Popover>
      </span>
    );
  }
}
