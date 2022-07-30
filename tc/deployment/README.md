# deployment






## deploy database

### deploy postgreSQL



CREATE USER $user_name WITH ENCRYPTED PASSWORD '$user_password';


CREATE DATABASE $db_name;
CREATE SCHEMA $db_schema;

GRANT ALL PRIVILEGES ON DATABASE $db_name to $user_name;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA $db_schema TO $user_name;
GRANT ALL PRIVILEGES ON $db_schema.global_txn_id_seq TO $user_name;


