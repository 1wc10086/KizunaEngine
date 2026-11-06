package shell

import (
	"context"

	"kizuna/kernel/translator"
)

var TranslationRunner func(ctx context.Context, req translator.Request, emit func(translator.Event)) error = func(ctx context.Context, req translator.Request, emit func(translator.Event)) error {
	return translator.Runner{}.Run(ctx, req, emit)
}
