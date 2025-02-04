// Copyright 2015 The go-ethereum Authors
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/pictor01/ALBA/alba/downloader"
	"github.com/pictor01/ALBA/alba/protocols/alba"
	"github.com/pictor01/ALBA/alba/protocols/snap"
	"github.com/pictor01/ALBA/p2p"
	"github.com/pictor01/ALBA/p2p/enode"
)

// Tests that snap sync is disabled after a successful sync cycle.
func TestSnapSyncDisabling66(t *testing.T) { testSnapSyncDisabling(t, alba.ALBA66, snap.SNAP1) }

// Tests that snap sync gets disabled as soon as a real block is successfully
// imported into the blockchain.
func testSnapSyncDisabling(t *testing.T, ethVer uint, snapVer uint) {
	t.Parallel()

	// Create an empty handler and ensure it's in snap sync mode
	empty := newTestHandler()
	if atomic.LoadUint32(&empty.handler.snapSync) == 0 {
		t.Fatalf("snap sync disabled on pristine blockchain")
	}
	defer empty.close()

	// Create a full handler and ensure snap sync ends up disabled
	full := newTestHandlerWithBlocks(1024)
	if atomic.LoadUint32(&full.handler.snapSync) == 1 {
		t.Fatalf("snap sync not disabled on non-empty blockchain")
	}
	defer full.close()

	// Sync up the two handlers via both `alba` and `snap`
	caps := []p2p.Cap{{Name: "eth", Version: ethVer}, {Name: "snap", Version: snapVer}}

	emptyPipeAlba, fullPipeEth := p2p.MsgPipe()
	defer emptyPipeAlba.Close()
	defer fullPipeAlba.Close()

	emptyPeerAlba := alba.NewPeer(ethVer, p2p.NewPeer(enode.ID{1}, "", caps), emptyPipeEth, empty.txpool)
	fullPeerAlba := alba.NewPeer(albaVer, p2p.NewPeer(enode.ID{2}, "", caps), fullPipeEth, full.txpool)
	defer emptyPeerAlba.Close()
	defer fullPeerAlba.Close()

	go empty.handler.runAlbaPeer(emptyPeerAlba, func(peer *alba.Peer) error {
		return alba.Handle((*albaHandler)(empty.handler), peer)
	})
	go full.handler.runAlbaPeer(fullPeerAlba, func(peer *alba.Peer) error {
		return alba.Handle((*albaHandler)(full.handler), peer)
	})

	emptyPipeSnap, fullPipeSnap := p2p.MsgPipe()
	defer emptyPipeSnap.Close()
	defer fullPipeSnap.Close()

	emptyPeerSnap := snap.NewPeer(snapVer, p2p.NewPeer(enode.ID{1}, "", caps), emptyPipeSnap)
	fullPeerSnap := snap.NewPeer(snapVer, p2p.NewPeer(enode.ID{2}, "", caps), fullPipeSnap)

	go empty.handler.runSnapExtension(emptyPeerSnap, func(peer *snap.Peer) error {
		return snap.Handle((*snapHandler)(empty.handler), peer)
	})
	go full.handler.runSnapExtension(fullPeerSnap, func(peer *snap.Peer) error {
		return snap.Handle((*snapHandler)(full.handler), peer)
	})
	// Wait a bit for the above handlers to start
	time.Sleep(250 * time.Millisecond)

	// Check that snap sync was disabled
	op := peerToSyncOp(downloader.SnapSync, empty.handler.peers.peerWithHighestTD())
	if err := empty.handler.doSync(op); err != nil {
		t.Fatal("sync failed:", err)
	}
	if atomic.LoadUint32(&empty.handler.snapSync) == 1 {
		t.Fatalf("snap sync not disabled after successful synchronisation")
	}
}
