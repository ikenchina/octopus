CREATE SCHEMA dtx;

CREATE TYPE dtx.txn_state AS ENUM (
    'prepared',
    'committed',
    'aborted'
);

CREATE TABLE IF NOT ExISTS dtx.rmtransaction(
	gtid character varying(32),
	bid INT NOT NULL,
	state dtx.txn_state DEFAULT 'prepared'::dtx.txn_state NOT NULL,
    CONSTRAINT gtid_bid_pk PRIMARY KEY(gtid, bid) 
);
