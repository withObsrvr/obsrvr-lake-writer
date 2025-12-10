package catalog

// Table schemas for the 19 DuckLake tables
// Based on ducklake-ingestion-obsrvr-v3 schemas

// GetTableSchema returns the DDL for a given table
func GetTableSchema(tableName string) string {
	schemas := map[string]string{
		"ledgers_row_v2": `
			sequence BIGINT NOT NULL,
			ledger_hash VARCHAR NOT NULL,
			previous_ledger_hash VARCHAR NOT NULL,
			closed_at TIMESTAMP NOT NULL,
			protocol_version INT NOT NULL,
			total_coins BIGINT NOT NULL,
			fee_pool BIGINT NOT NULL,
			base_fee INT NOT NULL,
			base_reserve INT NOT NULL,
			max_tx_set_size INT NOT NULL,
			successful_tx_count INT NOT NULL,
			failed_tx_count INT NOT NULL,
			ingestion_timestamp TIMESTAMP,
			ledger_range BIGINT,
			transaction_count INT,
			operation_count INT,
			tx_set_operation_count INT,
			soroban_fee_write1kb BIGINT,
			node_id VARCHAR,
			signature VARCHAR,
			ledger_header TEXT,
			bucket_list_size BIGINT,
			live_soroban_state_size BIGINT,
			evicted_keys_count INT,
			era_id VARCHAR,
			version_label VARCHAR
		`,

		"transactions_row_v2": `
			ledger_sequence BIGINT NOT NULL,
			transaction_hash VARCHAR NOT NULL,
			source_account VARCHAR NOT NULL,
			fee_charged BIGINT NOT NULL,
			max_fee BIGINT NOT NULL,
			successful BOOLEAN NOT NULL,
			transaction_result_code VARCHAR NOT NULL,
			operation_count INT NOT NULL,
			memo_type VARCHAR,
			memo VARCHAR,
			created_at TIMESTAMP NOT NULL,
			account_sequence BIGINT,
			ledger_range BIGINT,
			source_account_muxed VARCHAR,
			fee_account_muxed VARCHAR,
			inner_transaction_hash VARCHAR,
			fee_bump_transaction_hash VARCHAR,
			new_max_fee BIGINT,
			inner_signature_count INT,
			envelope_xdr TEXT,
			result_xdr TEXT,
			result_meta_xdr TEXT,
			fee_meta_xdr TEXT,
			signatures TEXT,
			time_bounds_start BIGINT,
			time_bounds_end BIGINT,
			ledger_bounds_min INT,
			ledger_bounds_max INT,
			min_account_sequence BIGINT,
			min_account_sequence_age BIGINT,
			min_account_sequence_ledger_gap BIGINT,
			extra_signers TEXT,
			resource_fee BIGINT,
			soroban_resources_instructions BIGINT,
			soroban_resources_read_bytes BIGINT,
			soroban_resources_write_bytes BIGINT,
			transaction_result_code_v2 VARCHAR,
			inclusion_fee_bid BIGINT,
			inclusion_fee_charged BIGINT,
			resource_fee_refund BIGINT,
			non_refundable_resource_fee_charged BIGINT,
			refundable_resource_fee_charged BIGINT,
			rent_fee_charged BIGINT,
			ext_v INT,
			era_id VARCHAR,
			version_label VARCHAR
		`,

		"operations_row_v2": `
			ledger_sequence BIGINT NOT NULL,
			transaction_hash VARCHAR NOT NULL,
			operation_index INT NOT NULL,
			type VARCHAR NOT NULL,
			source_account VARCHAR,
			successful BOOLEAN NOT NULL,
			operation_result_code VARCHAR,
			operation_trace_code VARCHAR,
			created_at TIMESTAMP NOT NULL,
			ledger_range BIGINT,
			source_account_muxed VARCHAR,
			destination_account VARCHAR,
			destination_account_muxed VARCHAR,
			asset_code VARCHAR,
			asset_issuer VARCHAR,
			asset_type VARCHAR,
			amount BIGINT,
			starting_balance BIGINT,
			account_merge_destination VARCHAR,
			path_asset_code VARCHAR,
			path_asset_issuer VARCHAR,
			send_asset_code VARCHAR,
			send_asset_issuer VARCHAR,
			send_amount BIGINT,
			dest_asset_code VARCHAR,
			dest_asset_issuer VARCHAR,
			dest_amount BIGINT,
			offer_id BIGINT,
			price_n INT,
			price_d INT,
			buying_asset_code VARCHAR,
			buying_asset_issuer VARCHAR,
			selling_asset_code VARCHAR,
			selling_asset_issuer VARCHAR,
			home_domain VARCHAR,
			low_threshold INT,
			med_threshold INT,
			high_threshold INT,
			master_weight INT,
			signer_key VARCHAR,
			signer_weight INT,
			authorize BOOLEAN,
			trustor VARCHAR,
			sponsor VARCHAR,
			sponsored_id VARCHAR,
			begin_sponsor VARCHAR,
			claimant VARCHAR,
			claimable_balance_id VARCHAR,
			liquidity_pool_id VARCHAR,
			liquidity_pool_type VARCHAR,
			reserve_a_asset_code VARCHAR,
			reserve_a_asset_issuer VARCHAR,
			reserve_a_amount BIGINT,
			reserve_b_asset_code VARCHAR,
			reserve_b_asset_issuer VARCHAR,
			reserve_b_amount BIGINT,
			era_id VARCHAR,
			version_label VARCHAR
		`,

		// Simplified schemas for other tables (can be expanded as needed)
		"effects_row_v1": `
			ledger_sequence BIGINT,
			transaction_hash VARCHAR,
			operation_index INT,
			type VARCHAR,
			created_at TIMESTAMP
		`,

		"trades_row_v1": `
			ledger_sequence BIGINT,
			trade_id VARCHAR,
			base_asset_code VARCHAR,
			counter_asset_code VARCHAR,
			created_at TIMESTAMP
		`,

		"accounts_snapshot_v1": `
			account_id VARCHAR,
			balance BIGINT,
			ledger_sequence BIGINT
		`,

		"trustlines_snapshot_v1": `
			account_id VARCHAR,
			asset_code VARCHAR,
			asset_issuer VARCHAR,
			balance BIGINT,
			ledger_sequence BIGINT
		`,

		"native_balances_snapshot_v1": `
			account_id VARCHAR NOT NULL,
			balance BIGINT NOT NULL,
			buying_liabilities BIGINT,
			selling_liabilities BIGINT,
			sequence_number BIGINT,
			num_subentries INT,
			num_sponsoring INT,
			num_sponsored INT,
			sponsor VARCHAR,
			last_modified_ledger BIGINT,
			ledger_entry_change INT,
			deleted BOOLEAN,
			ledger_range BIGINT
		`,

		"account_signers_snapshot_v1": `
			account_id VARCHAR,
			signer VARCHAR,
			weight INT,
			ledger_sequence BIGINT
		`,

		"contract_data_snapshot_v1": `
			contract_id VARCHAR,
			key VARCHAR,
			val TEXT,
			ledger_sequence BIGINT
		`,

		"contract_code_snapshot_v1": `
			hash VARCHAR,
			code BLOB,
			ledger_sequence BIGINT
		`,

		"contract_events_row_v1": `
			ledger_sequence BIGINT,
			transaction_hash VARCHAR,
			contract_id VARCHAR,
			topic_1 VARCHAR,
			data TEXT,
			created_at TIMESTAMP
		`,

		"config_settings_snapshot_v1": `
			config_setting_id VARCHAR,
			value TEXT,
			ledger_sequence BIGINT
		`,

		"ttl_snapshot_v1": `
			key_hash VARCHAR,
			live_until_ledger_seq BIGINT,
			ledger_sequence BIGINT
		`,

		"restored_keys_state_v1": `
			key_hash VARCHAR,
			ledger_sequence BIGINT,
			restored_at TIMESTAMP
		`,

		"evicted_keys_state_v1": `
			key_hash VARCHAR,
			ledger_sequence BIGINT,
			evicted_at TIMESTAMP
		`,

		"offers_snapshot_v1": `
			offer_id BIGINT,
			seller VARCHAR,
			selling_asset_code VARCHAR,
			buying_asset_code VARCHAR,
			amount BIGINT,
			ledger_sequence BIGINT
		`,

		"claimable_balances_snapshot_v1": `
			balance_id VARCHAR,
			asset_code VARCHAR,
			asset_issuer VARCHAR,
			amount BIGINT,
			ledger_sequence BIGINT
		`,

		"liquidity_pools_snapshot_v1": `
			liquidity_pool_id VARCHAR,
			asset_a_code VARCHAR,
			asset_b_code VARCHAR,
			reserves_a BIGINT,
			reserves_b BIGINT,
			ledger_sequence BIGINT
		`,
	}

	if ddl, ok := schemas[tableName]; ok {
		return ddl
	}

	// Default minimal schema if table not found
	return `
		id VARCHAR,
		data TEXT,
		ledger_sequence BIGINT,
		created_at TIMESTAMP
	`
}

// IsKnownTable checks if a table is one of the 19 known DuckLake tables
func IsKnownTable(tableName string) bool {
	knownTables := []string{
		"ledgers_row_v2",
		"transactions_row_v2",
		"operations_row_v2",
		"effects_row_v1",
		"trades_row_v1",
		"accounts_snapshot_v1",
		"trustlines_snapshot_v1",
		"native_balances_snapshot_v1",
		"account_signers_snapshot_v1",
		"contract_data_snapshot_v1",
		"contract_code_snapshot_v1",
		"contract_events_row_v1",
		"config_settings_snapshot_v1",
		"ttl_snapshot_v1",
		"restored_keys_state_v1",
		"evicted_keys_state_v1",
		"offers_snapshot_v1",
		"claimable_balances_snapshot_v1",
		"liquidity_pools_snapshot_v1",
	}

	for _, known := range knownTables {
		if tableName == known {
			return true
		}
	}
	return false
}
