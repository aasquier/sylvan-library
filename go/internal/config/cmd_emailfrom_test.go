package config

import "testing"

// A deployment that names its own From address keeps it, and stops calling
// itself the default.
//
// The one line of [LoadFrom] nothing ran. Every other test about the From
// address asks the opposite question — an unset variable falls to
// [DefaultEmailFrom], which is what a laptop wants — so the branch a deployed
// instance actually takes, the one that puts a real domain on every invite and
// every reset link, was never entered. The pair is what makes it worth
// asserting: the value lands *and* [Config.EmailFromIsDefault] stops saying
// yes, because that second answer is what the boot summary and the mail
// settings both branch on.
func TestAnInstanceThatNamesItsOwnFromAddressKeepsIt(t *testing.T) {
	t.Parallel()
	c := LoadFrom(environment(map[string]string{
		"MTGLAB_EMAIL_FROM": "  Sylvan Library <keeper@example.test>  ",
	}))
	if c.EmailFrom != "Sylvan Library <keeper@example.test>" {
		t.Errorf("the From address came through as %q", c.EmailFrom)
	}
	if c.EmailFromIsDefault() {
		t.Error("a chosen From address still reads as the built-in one")
	}
	// And nothing else moved: naming a sender is not a reconfiguration.
	if c.BaseURL != Defaults().BaseURL || c.DataDir != Defaults().DataDir {
		t.Errorf("setting one variable changed the rest: %+v", c)
	}
}
