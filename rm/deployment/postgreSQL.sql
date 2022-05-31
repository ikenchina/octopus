# CREATE SCHEMA dtx;

CREATE TYPE dtx.txn_state AS ENUM (
    'prepared',
    'committing',
    'committed',
    'failed',
    'aborted'
);


CREATE TABLE IF NOT ExISTS dtx.rmtransaction(
	gtid character varying(32) NOT NULL,
	branch_id INT NOT NULL,
	state dtx.txn_state DEFAULT 'prepared'::dtx.txn_state NOT NULL,
	body TEXT
);
