import _ from 'lodash';

export function normalizeFormData(values) {
  return {
    ...values,
    action: {
      converge: values.converge,
      recovery_notify: values.recovery_notify,
      notify_group: values.notify_group || undefined,
      notify_user: values.notify_user || undefined,
      callback: values.callback,
    },
    period_time: {
      enable_stime: values.enable_stime,
      enable_etime: values.enable_etime,
      enable_days_of_week: values.enable_days_of_week,
    },
    alert_upgrade: {
      enabled: !!values.need_upgrade,
      duration: _.get(values, 'alert_upgrade.duration', undefined),
      level: _.get(values, 'alert_upgrade.level', undefined),
      users: _.get(values, 'alert_upgrade.users', []),
      groups: _.get(values, 'alert_upgrade.groups', []),
    },
  };
}

export function processReqData(values) {
  const { action, period_time: periodTime } = values;
  const reqData = {
    ...values,
    ...action,
    ...periodTime,
    recovery_notify: values.recovery_notify ? 0 : 1,
    need_upgrade: _.get(values, 'alert_upgrade.enabled') ? 1 : 0,
    alert_upgrade: {
      duration: _.get(values, 'alert_upgrade.duration', undefined),
      level: _.get(values, 'alert_upgrade.level', undefined),
      users: _.get(values, 'alert_upgrade.users', []),
      groups: _.get(values, 'alert_upgrade.groups', []),
    },
  };
  delete reqData.action;
  delete reqData.period_time;
  return reqData;
}
