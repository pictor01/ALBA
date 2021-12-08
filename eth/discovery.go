// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package alba

import (
	"github.com/pictor01/ALBA/core"
	"github.com/pictor01/ALBA/core/forkid"
	"github.com/pictor01/ALBA/p2p/enode"
	"github.com/pictor01/ALBA/rlp"
)

// albaEntry is the "alba" ENR entry which advertises alba protocol
// on the discovery network.
type albaEntry struct {
	ForkID forkid.ID // Fork identifier per EIP-2124

	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}

// ENRKey implements enr.Entry.
func (e albaEntry) ENRKey() string {
	return "alba"
}

// startAlbaEntryUpdate starts the ENR updater loop.
func (alba *Alba) startAlbaEntryUpdate(ln *enode.LocalNode) {
	var newHead = make(chan core.ChainHeadEvent, 10)
	sub := alba.blockchain.SubscribeChainHeadEvent(newHead)

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-newHead:
				ln.Set(alba.currentEthEntry())
			case <-sub.Err():
				// Would be nice to sync with alba.Stop, but there is no
				// good way to do that.
				return
			}
		}
	}()
}

func (alba *Alba) currentAlbaEntry() *albaEntry {
	return &albaEntry{ForkID: forkid.NewID(alba.blockchain.Config(), alba.blockchain.Genesis().Hash(),
		alba.blockchain.CurrentHeader().Number.Uint64())}
}
