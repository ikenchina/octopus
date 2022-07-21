CREATE SCHEMA dtx;

CREATE TYPE dtx.txn_state AS ENUM (
    'prepared',
    'committing',
    'committed',
    'failed',
    'aborted'
);

CREATE TABLE IF NOT ExISTS dtx.rmtransaction(
    id BIGSERIAL PRIMARY key,
	gtid character varying(32) NOT NULL,
	bid INT NOT NULL,
	state dtx.txn_state DEFAULT 'prepared'::dtx.txn_state NOT NULL
);

CREATE UNIQUE INDEX gtid_bid_index ON dtx.rmtransaction(gtid, bid);
