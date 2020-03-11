import React from 'react';
import { Switch, Route, Redirect } from 'react-router-dom';
import PrivateRoute from '@cpts/Auth/PrivateRoute';
import Dashboard from './Dashboard';
import Tmpchart from './Tmpchart';
import Screen from './Screen';
import ScreenDetail from './Screen/ScreenDetail';
import Strategy from './Strategy';
import StrategyAdd from './Strategy/Add';
import StrategyModify from './Strategy/Modify';
import StrategyClone from './Strategy/Clone';
import Silence from './Silence';
import SilenceAdd from './Silence/Add';
import History from './History';
import HistoryDetail from './History/Detail';
import Collect from './Collect';
import CollectFormMain from './Collect/CollectFormMain';

export default function Routes() {
  const prePath = '/monitor';
  return (
    <Switch>
      <Route exact path={prePath} render={() => <Redirect to={`${prePath}/dashboard`} />} />
      <PrivateRoute exact path={`${prePath}/dashboard`} component={Dashboard} />
      <PrivateRoute exact path={`${prePath}/tmpchart`} component={Tmpchart} />
      <PrivateRoute exact path={`${prePath}/screen`} component={Screen} />
      <PrivateRoute exact path={`${prePath}/screen/:screenId`} component={ScreenDetail} />
      <PrivateRoute exact path={`${prePath}/history`} component={History} />
      <PrivateRoute exact path={`${prePath}/history/:historyType/:historyId`} component={HistoryDetail} />
      <PrivateRoute exact path={`${prePath}/strategy`} component={Strategy} />
      <PrivateRoute exact path={`${prePath}/strategy/add`} component={StrategyAdd} />
      <PrivateRoute exact path={`${prePath}/strategy/:strategyId/clone`} component={StrategyClone} />
      <PrivateRoute exact path={`${prePath}/strategy/:strategyId`} component={StrategyModify} />
      <PrivateRoute exact path={`${prePath}/silence`} component={Silence} />
      <PrivateRoute exact path={`${prePath}/silence/add`} component={SilenceAdd} />
      <PrivateRoute exact path={`${prePath}/collect`} component={Collect} />
      <PrivateRoute exact path={`${prePath}/collect/:action/:type`} component={CollectFormMain} />
      <PrivateRoute exact path={`${prePath}/collect/:action/:type/:id`} component={CollectFormMain} />
      <Route render={() => <Redirect to="/404" />} />
    </Switch>
  );
}
