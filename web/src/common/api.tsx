function getApi(path: string) {
  const prefix = '/api/portal';
  return `${prefix}${path}`;
}

const api = {
  login: getApi('/auth/login'),
  logout: getApi('/auth/logout'),
  selftProfile: getApi('/self/profile'),
  selftPassword: getApi('/self/password'),
  selftToken: getApi('/self/token'),
  user: getApi('/user'),
  users: getApi('/users'),
  team: getApi('/team'),
  tree: getApi('/tree'),
  treeSearch: getApi('/tree/search'),
  node: getApi('/node'),
  maskconf: getApi('/maskconf'),
  stra: getApi('/stra'),
  event: getApi('/event'),
  screen: getApi('/screen'),
  subclass: getApi('/subclass'),
  chart: getApi('/chart'),
  collect: getApi('/collect'),
  endpoint: getApi('/endpoint'),
  tmpchart: getApi('/tmpchart'),
  graphIndex: '/api/index',
  graphTransfer: '/api/transfer',
};

export default api;
