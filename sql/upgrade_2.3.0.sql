use n9e_mon;
ALTER TABLE plugin_collect ADD stdin text AFTER params;
ALTER TABLE plugin_collect ADD env text AFTER params;