package model

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"time"
)

type ProcessEventCommon struct {
	PageMadeTime       int64
	Title              string
	Namespace          string
	NamespaceId        int64
	Creator            string
	NumRecentEdits     int64
	NumRecentRevisions int64
}

type ProcessEventRevision struct {
	Timestamp int64
	Text      string `json:"-"`
	Id        int64
}

type ProcessEventUser struct {
	Username         string
	EditCount        int64
	DistinctPages    int64
	Warns            int64
	RegistrationTime int64
}

type ProcessEvent struct {
	Uuid           string
	ReceivedTime   time.Time
	Attempts       int32
	EditType       string
	EditId         int64
	User           ProcessEventUser
	Comment        string
	Length         int64
	PreviousUser   string
	Common         ProcessEventCommon
	Current        ProcessEventRevision
	Previous       ProcessEventRevision
	VandalismScore float32
	RevertReason   string
}

func (pe *ProcessEvent) FormatIrcRevert() string {
	return fmt.Sprintf("[[%s]] by \"%s\" (%s) %f",
		pe.TitleWithNamespace(),
		pe.User.Username,
		pe.GetDiffUrl(),
		pe.VandalismScore)
}

func (pe *ProcessEvent) TitleWithNamespace() string {
	if pe.Common.NamespaceId == 0 {
		return pe.Common.Title
	}
	return fmt.Sprintf("%s:%s", pe.Common.Namespace, pe.Common.Title)
}

func (pe *ProcessEvent) GetDiffUrl() string {
	return fmt.Sprintf("https://en.wikipedia.org/w/index.php?diff=%v&oldid=%v", pe.Current.Id, pe.Previous.Id)
}

func (pe *ProcessEvent) FormatIrcChange() string {
	return fmt.Sprintf("\x0314[[\x0307%v\x0314]]\x0304 \x0310 \x0302%v \x0305* \x0303%v \x0305* \x03(%v) \x0310%s\x03",
		pe.TitleWithNamespace(),
		pe.GetDiffUrl(),
		pe.User.Username,
		helpers.FormatPlusOrMinus(pe.Length),
		pe.Comment)
}
