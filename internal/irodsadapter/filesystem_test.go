package irodsadapter

import (
	"errors"
	"testing"

	"github.com/cyverse/go-irodsclient/irods/common"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

func TestIsUserNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "typed user not found",
			err:  irodstypes.NewUserNotFoundError("alice"),
			want: true,
		},
		{
			name: "catalog no rows",
			err:  irodstypes.NewIRODSError(common.CAT_NO_ROWS_FOUND),
			want: true,
		},
		{
			name: "other irods error",
			err:  irodstypes.NewIRODSError(common.CAT_INVALID_USER),
			want: false,
		},
		{
			name: "ordinary error",
			err:  errors.New("network failed"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUserNotFound(tt.err); got != tt.want {
				t.Fatalf("isUserNotFound() = %t, want %t", got, tt.want)
			}
		})
	}
}
