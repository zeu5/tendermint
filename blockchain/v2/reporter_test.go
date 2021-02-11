package v2

import (
	"sync"
	"testing"

	"github.com/tendermint/tendermint/p2p"
)

// TestMockReporter tests the MockReporter's ability to store reported
// peer behavior in memory indexed by the peerID.
func TestMockReporter(t *testing.T) {
	var peerID p2p.NodeID = "MockPeer"
	pr := NewMockReporter()

	behaviors := pr.GetBehaviors(peerID)
	if len(behaviors) != 0 {
		t.Error("Expected to have no behaviors reported")
	}

	badMessage := BadMessage(peerID, "bad message")
	if err := pr.Report(badMessage); err != nil {
		t.Error(err)
	}
	behaviors = pr.GetBehaviors(peerID)
	if len(behaviors) != 1 {
		t.Error("Expected the peer have one reported behavior")
	}

	if behaviors[0] != badMessage {
		t.Error("Expected Bad Message to have been reported")
	}
}

type scriptItem struct {
	peerID   p2p.NodeID
	behavior PeerBehavior
}

// equalBehaviors returns true if a and b contain the same PeerBehaviors with
// the same freequencies and otherwise false.
func equalBehaviors(a []PeerBehavior, b []PeerBehavior) bool {
	aHistogram := map[PeerBehavior]int{}
	bHistogram := map[PeerBehavior]int{}

	for _, behavior := range a {
		aHistogram[behavior]++
	}

	for _, behavior := range b {
		bHistogram[behavior]++
	}

	if len(aHistogram) != len(bHistogram) {
		return false
	}

	for _, behavior := range a {
		if aHistogram[behavior] != bHistogram[behavior] {
			return false
		}
	}

	for _, behavior := range b {
		if bHistogram[behavior] != aHistogram[behavior] {
			return false
		}
	}

	return true
}

// TestEqualPeerBehaviors tests that equalBehaviors can tell that two slices
// of peer behaviors can be compared for the behaviors they contain and the
// freequencies that those behaviors occur.
func TestEqualPeerBehaviors(t *testing.T) {
	var (
		peerID        p2p.NodeID = "MockPeer"
		consensusVote            = ConsensusVote(peerID, "voted")
		blockPart                = BlockPart(peerID, "blocked")
		equals                   = []struct {
			left  []PeerBehavior
			right []PeerBehavior
		}{
			// Empty sets
			{[]PeerBehavior{}, []PeerBehavior{}},
			// Single behaviors
			{[]PeerBehavior{consensusVote}, []PeerBehavior{consensusVote}},
			// Equal Frequencies
			{[]PeerBehavior{consensusVote, consensusVote},
				[]PeerBehavior{consensusVote, consensusVote}},
			// Equal frequencies different orders
			{[]PeerBehavior{consensusVote, blockPart},
				[]PeerBehavior{blockPart, consensusVote}},
		}
		unequals = []struct {
			left  []PeerBehavior
			right []PeerBehavior
		}{
			// Comparing empty sets to non empty sets
			{[]PeerBehavior{}, []PeerBehavior{consensusVote}},
			// Different behaviors
			{[]PeerBehavior{consensusVote}, []PeerBehavior{blockPart}},
			// Same behavior with different frequencies
			{[]PeerBehavior{consensusVote},
				[]PeerBehavior{consensusVote, consensusVote}},
		}
	)

	for _, test := range equals {
		if !equalBehaviors(test.left, test.right) {
			t.Errorf("expected %#v and %#v to be equal", test.left, test.right)
		}
	}

	for _, test := range unequals {
		if equalBehaviors(test.left, test.right) {
			t.Errorf("expected %#v and %#v to be unequal", test.left, test.right)
		}
	}
}

// TestPeerBehaviorConcurrency constructs a scenario in which
// multiple goroutines are using the same MockReporter instance.
// This test reproduces the conditions in which MockReporter will
// be used within a Reactor `Receive` method tests to ensure thread safety.
func TestMockPeerBehaviorReporterConcurrency(t *testing.T) {
	var (
		behaviorScript = []struct {
			peerID    p2p.NodeID
			behaviors []PeerBehavior
		}{
			{"1", []PeerBehavior{ConsensusVote("1", "")}},
			{"2", []PeerBehavior{ConsensusVote("2", ""), ConsensusVote("2", ""), ConsensusVote("2", "")}},
			{
				"3",
				[]PeerBehavior{BlockPart("3", ""),
					ConsensusVote("3", ""),
					BlockPart("3", ""),
					ConsensusVote("3", "")}},
			{
				"4",
				[]PeerBehavior{ConsensusVote("4", ""),
					ConsensusVote("4", ""),
					ConsensusVote("4", ""),
					ConsensusVote("4", "")}},
			{
				"5",
				[]PeerBehavior{BlockPart("5", ""),
					ConsensusVote("5", ""),
					BlockPart("5", ""),
					ConsensusVote("5", "")}},
		}
	)

	var receiveWg sync.WaitGroup
	pr := NewMockReporter()
	scriptItems := make(chan scriptItem)
	done := make(chan int)
	numConsumers := 3
	for i := 0; i < numConsumers; i++ {
		receiveWg.Add(1)
		go func() {
			defer receiveWg.Done()
			for {
				select {
				case pb := <-scriptItems:
					if err := pr.Report(pb.behavior); err != nil {
						t.Error(err)
					}
				case <-done:
					return
				}
			}
		}()
	}

	var sendingWg sync.WaitGroup
	sendingWg.Add(1)
	go func() {
		defer sendingWg.Done()
		for _, item := range behaviorScript {
			for _, reason := range item.behaviors {
				scriptItems <- scriptItem{item.peerID, reason}
			}
		}
	}()

	sendingWg.Wait()

	for i := 0; i < numConsumers; i++ {
		done <- 1
	}

	receiveWg.Wait()

	for _, items := range behaviorScript {
		reported := pr.GetBehaviors(items.peerID)
		if !equalBehaviors(reported, items.behaviors) {
			t.Errorf("expected peer %s to have behaved \nExpected: %#v \nGot %#v \n",
				items.peerID, items.behaviors, reported)
		}
	}
}
