// SPDX-License-Identifier: MIT

package names

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/muxrpc/v2"
)

type hGetSignifier struct {
	as  aboutStore
	log logging.Interface
}

func (h hGetSignifier) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	ref, err := parseFeedRefFromArgs(req)
	if err != nil {
		return nil, err
	}

	ai, err := h.as.CollectedFor(ref)
	if err != nil {
		return nil, fmt.Errorf("do not have about for: %s: %w", ref.Ref(), err)

	}
	var name = ai.Name.Chosen
	if name == "" {
		for n := range ai.Name.Prescribed { // pick random name
			name = n
			break
		}
		if name == "" {
			name = ref.Ref()
		}
	}

	return name, nil
}
