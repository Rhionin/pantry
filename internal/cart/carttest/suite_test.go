package carttest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/cart/carttest"
	_ "modernc.org/sqlite"
)

// capsSpread returns well-formed capability profiles covering every value of
// every dimension across the set, so RunContractSuite exercises each case
// function and NewFake builds each composite shape.
func capsSpread() []cart.Capabilities {
	return []cart.Capabilities{
		{Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush, Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived},
		{Auth: cart.AuthOAuth2, Delivery: cart.DeliveryServerPush, Confirmation: cart.ConfirmPerLine, Mutation: cart.MutateAddAndUpdate, Identity: cart.IdentityLookedUp},
		{Auth: cart.AuthAPIKey, Delivery: cart.DeliveryClientHandoff, Confirmation: cart.ConfirmNone, Mutation: cart.MutateFull, Identity: cart.IdentityDerived},
		{Auth: cart.AuthOAuth2, Delivery: cart.DeliveryClientHandoff, Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateFull, Identity: cart.IdentityLookedUp},
		{Auth: cart.AuthAPIKey, Delivery: cart.DeliveryServerPush, Confirmation: cart.ConfirmPerLine, Mutation: cart.MutateAddAndUpdate, Identity: cart.IdentityDerived},
		{Auth: cart.AuthNone, Delivery: cart.DeliveryClientHandoff, Confirmation: cart.ConfirmNone, Mutation: cart.MutateAddOnly, Identity: cart.IdentityLookedUp},
	}
}

// A well-formed fake built by NewFake passes the contract suite. The suite fails
// a provider whose declared dimension has no matching interface, so a pass
// proves the composite satisfies exactly the interfaces its caps declare.
func TestRunContractSuite_PassesForWellFormedFakes(t *testing.T) {
	for _, caps := range capsSpread() {
		caps := caps
		name := string(caps.Auth) + "/" + string(caps.Delivery) + "/" + string(caps.Identity)
		t.Run(name, func(t *testing.T) {
			fake := carttest.NewFake(caps, carttest.NewScript())
			carttest.RunContractSuite(t, fake)
		})
	}
}

// NewFake wires each capability dimension to its narrow interface, and only
// those. The auth dimension is exclusive: oauth2 gives OAuthFlow and not
// StaticCredential, api_key the reverse, none neither.
func TestNewFake_SatisfiesExactlyDeclaredInterfaces(t *testing.T) {
	script := carttest.NewScript()

	oauthPush := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthOAuth2, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateFull, Identity: cart.IdentityLookedUp,
	}, script)

	if _, ok := oauthPush.(cart.OAuthFlow); !ok {
		t.Error("oauth2 fake does not satisfy OAuthFlow")
	}
	if _, ok := oauthPush.(cart.StaticCredential); ok {
		t.Error("oauth2 fake unexpectedly satisfies StaticCredential")
	}
	if _, ok := oauthPush.(cart.ServerPush); !ok {
		t.Error("server_push fake does not satisfy ServerPush")
	}
	if _, ok := oauthPush.(cart.HandoffBuilder); ok {
		t.Error("server_push fake unexpectedly satisfies HandoffBuilder")
	}
	if _, ok := oauthPush.(cart.LookedUpIdentity); !ok {
		t.Error("looked_up fake does not satisfy LookedUpIdentity")
	}
	if _, ok := oauthPush.(cart.DerivedIdentity); ok {
		t.Error("looked_up fake unexpectedly satisfies DerivedIdentity")
	}

	apiKeyHandoff := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthAPIKey, Delivery: cart.DeliveryClientHandoff,
		Confirmation: cart.ConfirmNone, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)

	if _, ok := apiKeyHandoff.(cart.StaticCredential); !ok {
		t.Error("api_key fake does not satisfy StaticCredential")
	}
	if _, ok := apiKeyHandoff.(cart.OAuthFlow); ok {
		t.Error("api_key fake unexpectedly satisfies OAuthFlow")
	}
	if _, ok := apiKeyHandoff.(cart.HandoffBuilder); !ok {
		t.Error("client_handoff fake does not satisfy HandoffBuilder")
	}
	if _, ok := apiKeyHandoff.(cart.ServerPush); ok {
		t.Error("client_handoff fake unexpectedly satisfies ServerPush")
	}
	if _, ok := apiKeyHandoff.(cart.DerivedIdentity); !ok {
		t.Error("derived fake does not satisfy DerivedIdentity")
	}
	if _, ok := apiKeyHandoff.(cart.LookedUpIdentity); ok {
		t.Error("derived fake unexpectedly satisfies LookedUpIdentity")
	}

	noneFake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)
	if _, ok := noneFake.(cart.OAuthFlow); ok {
		t.Error("auth=none fake unexpectedly satisfies OAuthFlow")
	}
	if _, ok := noneFake.(cart.StaticCredential); ok {
		t.Error("auth=none fake unexpectedly satisfies StaticCredential")
	}
}

func TestFake_ServerPush_Add_ReturnsScriptedDisposition(t *testing.T) {
	script := carttest.NewScript().
		WithDispositions(cart.DispositionRejected).
		WithLineResults(map[cart.ProductIdentity]bool{"sku-1": true})

	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerLine, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)

	push := fake.(cart.ServerPush)
	result, err := push.Add(context.Background(), cart.NoCredential{}, cart.ProvisionRequest{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if result.Disposition != cart.DispositionRejected {
		t.Errorf("Disposition = %q, want %q", result.Disposition, cart.DispositionRejected)
	}
	if got := result.PerLine["sku-1"]; got != true {
		t.Errorf("PerLine[sku-1] = %v, want true", got)
	}
	if result.Status != 200 {
		t.Errorf("Status = %d, want 200", result.Status)
	}
	assertCalls(t, fake, []string{"Add"})
}

// With no scripted dispositions, Add falls through to the accepted default.
func TestFake_ServerPush_Add_DefaultsToAccepted(t *testing.T) {
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, carttest.NewScript())

	result, err := fake.(cart.ServerPush).Add(context.Background(), cart.NoCredential{}, cart.ProvisionRequest{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if result.Disposition != cart.DispositionAccepted {
		t.Errorf("Disposition = %q, want %q", result.Disposition, cart.DispositionAccepted)
	}
}

func TestFake_DeriveIdentity_ReturnsScriptedLookup(t *testing.T) {
	script := carttest.NewScript().WithIdentityLookup("012345", "internal-sku")
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)

	derived := fake.(cart.DerivedIdentity)

	identity, ok := derived.DeriveIdentity("012345")
	if !ok {
		t.Fatal("DeriveIdentity(012345) ok = false, want true")
	}
	if identity != "internal-sku" {
		t.Errorf("identity = %q, want %q", identity, "internal-sku")
	}

	if _, ok := derived.DeriveIdentity("nope"); ok {
		t.Error("DeriveIdentity(nope) ok = true, want false")
	}
	assertCalls(t, fake, []string{"DeriveIdentity", "DeriveIdentity"})
}

func TestFake_LookUpIdentity_ReturnsScriptedLookup(t *testing.T) {
	script := carttest.NewScript().WithIdentityLookup("999", "remote-id")
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthAPIKey, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityLookedUp,
	}, script)

	lookedUp := fake.(cart.LookedUpIdentity)

	identity, ok, err := lookedUp.LookUpIdentity(context.Background(), cart.NoCredential{}, "999")
	if err != nil {
		t.Fatalf("LookUpIdentity: %v", err)
	}
	if !ok {
		t.Fatal("LookUpIdentity(999) ok = false, want true")
	}
	if identity != "remote-id" {
		t.Errorf("identity = %q, want %q", identity, "remote-id")
	}

	if _, ok, _ := lookedUp.LookUpIdentity(context.Background(), cart.NoCredential{}, "absent"); ok {
		t.Error("LookUpIdentity(absent) ok = true, want false")
	}
	assertCalls(t, fake, []string{"LookUpIdentity", "LookUpIdentity"})
}

func TestFake_ExchangeCode_Success(t *testing.T) {
	script := carttest.NewScript().WithTokenExchangeSuccess()
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthOAuth2, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)

	tokens, err := fake.(cart.OAuthFlow).ExchangeCode(context.Background(), "auth-code")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tokens.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "test-access-token")
	}
	if tokens.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, "test-refresh-token")
	}
	assertCalls(t, fake, []string{"ExchangeCode"})
}

func TestFake_ExchangeCode_Error(t *testing.T) {
	wantErr := errors.New("invalid_grant")
	script := carttest.NewScript().WithTokenExchangeError(wantErr)
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthOAuth2, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)

	_, err := fake.(cart.OAuthFlow).ExchangeCode(context.Background(), "bad-code")
	if !errors.Is(err, wantErr) {
		t.Errorf("ExchangeCode err = %v, want %v", err, wantErr)
	}
}

func TestFake_RefreshAccessToken_Success(t *testing.T) {
	script := carttest.NewScript().WithTokenExchangeSuccess()
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthOAuth2, Delivery: cart.DeliveryClientHandoff,
		Confirmation: cart.ConfirmNone, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)

	tokens, err := fake.(cart.OAuthFlow).RefreshAccessToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	if tokens.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "test-access-token")
	}
	assertCalls(t, fake, []string{"RefreshAccessToken"})
}

// AuthorizationURL embeds the state parameter and AuthorizationScope is fixed.
func TestFake_OAuthFlow_AuthorizationURLAndScope(t *testing.T) {
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthOAuth2, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, carttest.NewScript())

	flow := fake.(cart.OAuthFlow)
	url, err := flow.AuthorizationURL("xyz-state")
	if err != nil {
		t.Fatalf("AuthorizationURL: %v", err)
	}
	if url != "https://example.com/auth?xyz-state" {
		t.Errorf("AuthorizationURL = %q", url)
	}
	if scope := flow.AuthorizationScope(); scope != "test:scope" {
		t.Errorf("AuthorizationScope = %q, want %q", scope, "test:scope")
	}
}

// StaticCredential.Credential returns a bearer credential that redacts in String.
func TestFake_StaticCredential_ReturnsRedactedCredential(t *testing.T) {
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthAPIKey, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, carttest.NewScript())

	cred := fake.(cart.StaticCredential).Credential()
	if cred.String() != "bearer(redacted)" {
		t.Errorf("Credential().String() = %q, want %q", cred.String(), "bearer(redacted)")
	}
	assertCalls(t, fake, []string{"Credential"})
}

func TestFake_BuildHandoff_ReturnsArtifact(t *testing.T) {
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryClientHandoff,
		Confirmation: cart.ConfirmNone, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, carttest.NewScript())

	artifact, err := fake.(cart.HandoffBuilder).BuildHandoff(cart.ProvisionRequest{})
	if err != nil {
		t.Fatalf("BuildHandoff: %v", err)
	}
	if artifact.URL != "https://example.com/handoff" {
		t.Errorf("URL = %q", artifact.URL)
	}
	if artifact.Data["key"] != "value" {
		t.Errorf("Data[key] = %q, want %q", artifact.Data["key"], "value")
	}
	assertCalls(t, fake, []string{"BuildHandoff"})
}

func TestFake_UpdateLine_ReturnsAccepted(t *testing.T) {
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddAndUpdate, Identity: cart.IdentityDerived,
	}, carttest.NewScript())

	result, err := fake.(cart.LineUpdater).UpdateLine(context.Background(), cart.NoCredential{}, cart.ProvisionLine{Identity: "sku-1", Quantity: 2})
	if err != nil {
		t.Fatalf("UpdateLine: %v", err)
	}
	if result.Disposition != cart.DispositionAccepted {
		t.Errorf("Disposition = %q, want %q", result.Disposition, cart.DispositionAccepted)
	}
	assertCalls(t, fake, []string{"UpdateLine"})
}

func TestFake_RemoveLine_ReturnsAccepted(t *testing.T) {
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateFull, Identity: cart.IdentityDerived,
	}, carttest.NewScript())

	result, err := fake.(cart.LineRemover).RemoveLine(context.Background(), cart.NoCredential{}, "sku-9")
	if err != nil {
		t.Fatalf("RemoveLine: %v", err)
	}
	if result.Disposition != cart.DispositionAccepted {
		t.Errorf("Disposition = %q, want %q", result.Disposition, cart.DispositionAccepted)
	}
	assertCalls(t, fake, []string{"RemoveLine"})
}

// A sequence of scripted dispositions is consumed one per Add call, in order.
func TestFake_ServerPush_ConsumesDispositionsInOrder(t *testing.T) {
	script := carttest.NewScript().WithDispositions(
		cart.DispositionAccepted, cart.DispositionIndeterminate, cart.DispositionRejected,
	)
	fake := carttest.NewFake(cart.Capabilities{
		Auth: cart.AuthNone, Delivery: cart.DeliveryServerPush,
		Confirmation: cart.ConfirmPerRequest, Mutation: cart.MutateAddOnly, Identity: cart.IdentityDerived,
	}, script)
	push := fake.(cart.ServerPush)

	want := []cart.ResultDisposition{
		cart.DispositionAccepted, cart.DispositionIndeterminate, cart.DispositionRejected, cart.DispositionAccepted,
	}
	for i, w := range want {
		result, err := push.Add(context.Background(), cart.NoCredential{}, cart.ProvisionRequest{})
		if err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
		if result.Disposition != w {
			t.Errorf("Add #%d Disposition = %q, want %q", i, result.Disposition, w)
		}
	}
}

func assertCalls(t *testing.T, p cart.Provider, want []string) {
	t.Helper()
	recorder, ok := p.(carttest.Calls)
	if !ok {
		t.Fatalf("provider %T does not expose Calls()", p)
	}
	got := recorder.Calls()
	if len(got) != len(want) {
		t.Fatalf("Calls() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Calls()[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
}
