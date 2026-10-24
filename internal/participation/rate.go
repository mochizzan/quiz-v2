package participation

// rate.go is PAR's slice of the shared limiter: the protected action
// families and their per-window budgets (§8.7.8). The limiter itself and its
// storage are IDN's; PAR only declares bucket names and budgets so no two
// domains ever share a key. The adapters call Limiter.Allow with these keys
// before any lookup or write; a 429 answers R-19 copy and never touches a
// cookie, so a later retry keeps its ticket (§8.7.3, §9.4.3).

import (
	"time"
)

// RateWindow is the fixed 15-minute window every PAR bucket uses.
const RateWindow = 15 * time.Minute

// Per-window budgets, one per closed action family (§8.7.8).
const (
	RateJoinLimit     = 30 // POST /join code resolution
	RateIdentifyLimit = 10 // POST /join/identify
	RateRegisterLimit = 20 // POST /quiz/:id/register
	RateCancelLimit   = 20 // POST /quiz/:id/cancel
	RateStartLimit    = 40 // POST /quiz/:id/attempts
	RateManageLimit   = 40 // teacher management writes
)

// RateKey namespaces one bucket ("par:<what>:<identity>"). Public actions are
// IP-keyed; teacher management is account-keyed so one teacher hammering a
// form does not punish a colleague.
func RateKey(what, identity string) string { return "par:" + what + ":" + identity }
