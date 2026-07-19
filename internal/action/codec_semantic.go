package action

import (
	"encoding/json"
	"fmt"
)

var scrollToPromptCodec = codecOps{
	encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
		value, ok := action.(ScrollToPrompt)
		if !ok {
			return nil, fmt.Errorf("expected ScrollToPrompt, got %T", action)
		}
		return json.Marshal(scrollToPromptArgs{Delta: value.Delta})
	},
	decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
		var args scrollToPromptArgs
		if err := decodeObject(data, &args); err != nil {
			return nil, err
		}
		return ScrollToPrompt{Delta: args.Delta}, nil
	},
}
