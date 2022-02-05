# CREATE SCHEMA dtx;

CREATE TYPE txn_state AS ENUM (
    'prepared',
    'committing',
    'committed',
    'failed',
    'aborted'
);


CREATE TABLE IF NOT ExISTS rmtransaction(
	gtid character varying(32) NOT NULL,
	branch_id INT NOT NULL,
	state txn_state DEFAULT 'prepared'::txn_state NOT NULL,
	body TEXT
);
