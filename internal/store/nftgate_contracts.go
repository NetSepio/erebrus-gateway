package store

import (
	"context"
	"fmt"
)

// NFTGateContract is a chain-specific collection/contract that grants NFT entitlement.
type NFTGateContract struct {
	ID      string
	Chain   string
	Address string
	Label   string
}

// ListNFTGateContracts returns enabled gating contracts ordered by chain, address.
func (s *Store) ListNFTGateContracts(ctx context.Context) ([]NFTGateContract, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, chain, address, COALESCE(label, '')
		FROM nft_gate_contracts
		WHERE enabled = true
		ORDER BY chain, address`)
	if err != nil {
		return nil, fmt.Errorf("list nft gate contracts: %w", err)
	}
	defer rows.Close()

	var out []NFTGateContract
	for rows.Next() {
		var c NFTGateContract
		if err := rows.Scan(&c.ID, &c.Chain, &c.Address, &c.Label); err != nil {
			return nil, fmt.Errorf("scan nft gate contract: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}