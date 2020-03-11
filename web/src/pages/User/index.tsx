import React from 'react';
import { Switch, Route, Redirect } from 'react-router-dom';
import PrivateRoute from '@cpts/Auth/PrivateRoute';
import List from './List';
import Team from './Team';


export default function Routes() {
  const prePath = '/user';
  return (
    <Switch>
      <Route exact path={prePath} render={() => <Redirect to={`${prePath}/list`} />} />
      <PrivateRoute path={`${prePath}/list`} component={List} />
      <PrivateRoute path={`${prePath}/team`} component={Team} />
      <Route render={() => <Redirect to="/404" />} />
    </Switch>
  );
}
