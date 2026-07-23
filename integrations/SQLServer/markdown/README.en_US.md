# sqlserver

forked from telegraf/sqlserver. This plugin collects sqlserver monitoring metrics. The Azure-related monitoring parts have been removed, keeping only the on-premises sqlserver deployment scenario.

# Usage
Create a monitoring account as follows, to be used for reading monitoring data:
USE master;

CREATE LOGIN [categraf] WITH PASSWORD = N'mystrongpassword';

GRANT VIEW SERVER STATE TO [categraf];

GRANT VIEW ANY DEFINITION TO [categraf];
Data Source=10.19.1.1;Initial Catalog=hc;User ID=sa;Password=mystrongpassword;
