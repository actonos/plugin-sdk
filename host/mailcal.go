package host

import (
	"context"
	"fmt"

	"github.com/actonos/plugin-sdk/sdk/mailcal"
)

// CallJSON invokes one mail/calendar ABI export with a JSON request and returns the JSON
// response. It satisfies mailcal.Caller, so mailcal.NewBridgeProvider(runner) gives a typed view.
//
// The guest releases the request buffer itself on its next call, so nothing is freed here.
func (r *PluginRunner) CallJSON(ctx context.Context, export string, req []byte) ([]byte, error) {
	fn := r.mod.ExportedFunction(export)
	if fn == nil {
		return nil, fmt.Errorf("%s: %w", export, mailcal.ErrFunctionNotFound)
	}
	ptr, n, err := r.writeBytes(req)
	if err != nil {
		return nil, err
	}
	results, err := fn.Call(ctx, uint64(ptr), uint64(n))
	if err != nil {
		return nil, fmt.Errorf("calling %s: %w", export, err)
	}
	if len(results) == 0 || results[0] == 0 {
		return nil, fmt.Errorf("%s returned no data", export)
	}
	return r.readBytes(uint32(results[0]>>32), uint32(results[0]&0xFFFFFFFF))
}

var _ mailcal.Caller = (*PluginRunner)(nil)
