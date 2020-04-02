import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Row, Col, Button, Icon, message, Popconfirm } from 'antd';
import _ from 'lodash';
import { FormattedMessage, injectIntl } from 'react-intl';
import Filter from './Filter';
import filterFormModal from './FilterFormModal';
import './style.less';

const commonPropTypes = {
  value: PropTypes.array,
  onChange: PropTypes.func,
  readOnly: PropTypes.bool,
  tags: PropTypes.object,
};

const commonPropDefaultValue = {
  readOnly: false,
  tags: {},
};

class Filters extends Component {
  static propTypes = {
    ...commonPropTypes,
  };

  static defaultProps = {
    ...commonPropDefaultValue,
  };

  addFilter = () => {
    const { tags, value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    filterFormModal({
      language: this.props.intl.locale,
      title: <FormattedMessage id="stra.tag.add" />,
      tags,
      onOk: (data) => {
        if (!_.find(value, { tkey: data.tkey })) {
          valueClone.push(data);
          onChange(valueClone);
        } else {
          message.warning('该 Tag 已存在，请更改 Tag');
        }
      },
    });
  }

  updateFilter = (val) => {
    const { tags, value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    filterFormModal({
      language: this.props.intl.locale,
      title: <FormattedMessage id="stra.tag.modify" />,
      tags,
      data: val,
      onOk: (data) => {
        if (!_.find(value, { tkey: data.tkey }) || val.tkey === data.tkey) {
          _.remove(valueClone, o => o.tkey === val.tkey);
          valueClone.push(data);
          onChange(valueClone);
        } else {
          message.warning('该 Tag 已存在，请更改 Tag');
        }
      },
    });
  }

  deleteFilter = (val) => {
    const { value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    _.remove(valueClone, o => o.tkey === val.tkey);
    onChange(valueClone);
  }

  renderFilters() {
    const { readOnly, value } = this.props;
    const filtersEle = _.map(value, (item, i) => {
      return (
        <Col span={12} key={i} style={{ marginTop: 5 }}>
          <Filter
            data={item}
            extra={
              !readOnly &&
              <span className="strategy-filter-operation">
                <Icon type="edit" onClick={() => this.updateFilter(item)} />
                <Popconfirm title={<FormattedMessage id="table.delete.sure" />} onConfirm={() => this.deleteFilter(item)}>
                  <Icon type="cross" />
                </Popconfirm>
              </span>
            }
          />
        </Col>
      );
    });

    return <Row gutter={10}>{ filtersEle }</Row>;
  }

  render() {
    const { readOnly, value } = this.props;

    if (readOnly) {
      return (
        <div className="strategy-filters">
          {this.renderFilters()}
        </div>
      );
    }
    return (
      <div className="strategy-filters">
        <Button
          type="ghost"
          size="default"
          onClick={this.addFilter}
        >
          <FormattedMessage id="stra.tag.add" />
        </Button>
        {
          value.length ? this.renderFilters() : null
        }
      </div>
    );
  }
}

export default injectIntl(Filters);
