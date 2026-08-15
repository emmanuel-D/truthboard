#!/usr/bin/env sh
# Build the acme-shop demo repository the README's screenshots are taken from.
#
#   docs/demo/build-demo-repo.sh [target-dir]     # default: /tmp/acme-shop
#
# Everything the board shows is derived from what this script writes: branches,
# merges, commit trailers and story files. Nothing sets a status, here or
# anywhere else — the states below are the *shape of history* that produces
# them, which is the whole point of the demo.
#
# Dates are relative to the day you run it, so a rebuild always looks live:
# sprint s12 is mid-flight, the stalled story stopped last week, the work in
# flight has commits from this morning.
#
# One derived status is deliberately missing. `in-review` comes from an open
# pull request, read from a forge with `gh`/`glab` — a local repository has no
# forge to read, so a demo that showed it would be showing a lie.

set -eu

TARGET="${1:-/tmp/acme-shop}"

# days_ago N -> an ISO timestamp N days back, for GIT_*_DATE.
days_ago() {
	if date -v -1d >/dev/null 2>&1; then	# BSD/macOS
		date -u -v "-$1d" +"%Y-%m-%dT%H:%M:%SZ"
	else					# GNU
		date -u -d "$1 days ago" +"%Y-%m-%dT%H:%M:%SZ"
	fi
}

# day_offset ±N -> an ISO date N days from today. The sign is required: BSD
# date reads an unsigned adjustment as an absolute field, and silently.
day_offset() {
	case "$1" in
	-*) signed="$1" ;;
	*) signed="+$1" ;;
	esac
	if date -v -1d >/dev/null 2>&1; then	# BSD/macOS
		date -u -v "${signed}d" +"%Y-%m-%d"
	else					# GNU
		date -u -d "$signed days" +"%Y-%m-%d"
	fi
}

# commit <days-ago> <message...> — commits the index with a backdated timestamp
# so cycle time, staleness and the digest window have something real to measure.
commit() {
	when="$(days_ago "$1")"
	shift
	GIT_AUTHOR_DATE="$when" GIT_COMMITTER_DATE="$when" \
		git commit -q --no-verify -m "$@"
}

if [ -e "$TARGET" ]; then
	printf 'refusing to overwrite %s — remove it first\n' "$TARGET" >&2
	exit 1
fi

mkdir -p "$TARGET"
cd "$TARGET"
git init -q -b main
git config user.name "Acme Engineering"
git config user.email "eng@acme.example"
git config commit.gpgsign false

mkdir -p src/checkout src/payments src/activation .truthboard/specs .truthboard/sprints

cat >README.md <<'EOF'
# acme-shop

The storefront. Checkout, payments, and the emails that follow an order.

Tracked with [Truthboard](https://github.com/emmanuel-D/truthboard): the
board is derived from this repository, so nothing in it can be typed.
EOF

cat >src/checkout/basket.js <<'EOF'
export function total(items) {
  return items.reduce((sum, i) => sum + i.price * i.qty, 0);
}
EOF

cat >src/payments/gateway.js <<'EOF'
export async function charge(amount, method) {
  return { ok: true, amount, method };
}
EOF

git add -A
commit 60 "The storefront, as it was"

# ---------------------------------------------------------------------------
# Sprint intent. Dates are the only thing a sprint carries; whether it is
# future, active or completed is arithmetic over them and today.
# ---------------------------------------------------------------------------

cat >.truthboard/sprints/s12.md <<EOF
---
slug: s12
start: $(day_offset -6)
end: $(day_offset 5)
---

Checkout has to survive Black Friday. Everything else waits.
EOF

cat >.truthboard/sprints/s13.md <<EOF
---
slug: s13
start: $(day_offset 6)
end: $(day_offset 17)
---

Returning customers: saved cards, and the reminder that brings them back.
EOF

# ---------------------------------------------------------------------------
# The backlog. Every one of these is intent — a promise somebody made. The
# board will not believe a word of it until git says so.
# ---------------------------------------------------------------------------

spec() { cat >".truthboard/specs/$1.md"; }

spec tb-4f2a-one-page-checkout <<'EOF'
---
id: tb-4f2a
title: One-page checkout flow
owner: maya
branch: '*/tb-4f2a-*'
epic: checkout
sprint: s12
priority: 1
type: story
points: 8
---

## Goal

Our checkout is four pages and we lose a quarter of the basket on every
one of them. Collapse it into a single page that keeps address, delivery
and payment on screen together, so a customer can see what they are
agreeing to without navigating away from it.

## Acceptance

- [x] A customer completes an order without a page navigation — proof: `src/checkout/onepage.js`
- [x] Address, delivery and payment validate inline, not on submit
- [x] The basket total is visible at every step
- [x] Going back never loses entered details
- [ ] Conversion is measured against the four-page flow for two weeks
EOF

spec tb-7c31-apple-pay <<'EOF'
---
id: tb-7c31
title: Apple Pay as a payment method
owner: dev
branch: '*/tb-7c31-*'
epic: payments
sprint: s12
priority: 1
type: story
points: 5
---

## Goal

Half our traffic is iOS and every one of those customers types a card
number by hand. Offer Apple Pay at checkout so the ones who have already
told their phone their card details do not have to tell us as well.

## Acceptance

- [x] The Apple Pay button appears only where the browser supports it
- [ ] A completed sheet produces the same order as a card payment
- [ ] A cancelled sheet returns to checkout with the basket intact
- [ ] Merchant validation is server-side; no certificate reaches the client
EOF

spec tb-2b9e-declined-card-basket <<'EOF'
---
id: tb-2b9e
title: A declined card empties the basket
owner: sam
branch: '*/tb-2b9e-*'
epic: checkout
sprint: s12
priority: 1
type: bug
points: 3
---

## Goal

When the gateway declines a card we redirect to an error page, and the
session that held the basket does not survive the round trip. The
customer with a second card in their wallet has nothing left to pay for.

## Acceptance

- [ ] A declined payment returns to checkout with every item still in the basket
- [ ] The decline reason is shown in the customer's words, not the gateway's code
- [ ] Retrying with a second card completes the same order, not a new one
EOF

spec tb-9a15-guest-checkout <<'EOF'
---
id: tb-9a15
title: Guest checkout without an account
owner: priya
branch: '*/tb-9a15-*'
epic: checkout
sprint: s12
priority: 2
type: story
points: 5
---

## Goal

Creating an account is the last thing someone wants to do while holding a
credit card. Let a customer order with an email address alone, and offer
the account afterwards, when it is a convenience rather than a toll.

## Acceptance

- [ ] An order completes with an email address and no password
- [ ] The confirmation page offers an account, and declining it keeps the order
- [ ] A guest email that matches an existing account does not silently merge them
EOF

spec tb-c8d4-welcome-email <<'EOF'
---
id: tb-c8d4
title: Welcome email on first order
owner: maya
branch: '*/tb-c8d4-*'
epic: activation
sprint: s12
priority: 2
type: story
points: 3
hold: waiting on legal sign-off for the copy
---

## Goal

A first order is the one moment a customer is definitely paying attention.
Send one email that says what they bought, when it arrives, and how to
reach a human — and nothing else.

## Acceptance

- [ ] A first order sends exactly one welcome email
- [ ] A second order sends none
- [ ] The email renders in Gmail, Outlook and Apple Mail
EOF

spec tb-5e77-saved-cards <<'EOF'
---
id: tb-5e77
title: Saved cards for returning customers
owner: dev
branch: '*/tb-5e77-*'
epic: payments
sprint: s13
priority: 1
type: story
points: 8
needs:
    - tb-7c31
---

## Goal

A returning customer should not retype a card we are already allowed to
charge. Store the gateway's token — never the number — and offer it at
checkout behind the same confirmation a new card gets.

## Acceptance

- [ ] A saved card completes an order without re-entering the number
- [ ] No card number, CVV or expiry is ever stored by us
- [ ] Removing a saved card revokes the token at the gateway too
EOF

spec tb-1d38-confirmation-total <<'EOF'
---
id: tb-1d38
title: The confirmation page shows the pre-discount total
owner: sam
branch: '*/tb-1d38-*'
epic: checkout
sprint: s12
priority: 1
type: bug
points: 2
---

## Goal

The order is charged correctly and the confirmation page reports the
total before the discount was applied. Support gets the call, and the
customer has every reason to believe the higher number.

## Acceptance

- [ ] The confirmation total matches the amount charged, discounts included
- [ ] The discount is shown as its own line
EOF

spec tb-3e92-payments-sdk <<'EOF'
---
id: tb-3e92
title: Upgrade the payments SDK past its EOL
owner: dev
branch: '*/tb-3e92-*'
epic: payments
sprint: s12
priority: 1
type: task
points: 2
---

## Goal

The gateway drops support for v2 at the end of the quarter. Move to v4
now, while it is a scheduled afternoon rather than an outage.

## Acceptance

- [x] Every call site uses v4
- [x] Payment tests pass against the v4 sandbox
EOF

spec tb-b6f0-abandoned-basket <<'EOF'
---
id: tb-b6f0
title: Abandoned-basket reminder
owner: priya
branch: '*/tb-b6f0-*'
epic: activation
sprint: s13
priority: 2
type: story
points: 5
---

## Goal

Most baskets are abandoned mid-checkout and never mentioned again. Send
one reminder, a day later, with the basket still intact behind the link —
and never a second one.

## Acceptance

- [ ] An abandoned basket sends one reminder after 24 hours
- [ ] The link restores the basket exactly as it was left
- [ ] Completing an order cancels a pending reminder
EOF

spec tb-8a4c-address-autocomplete <<'EOF'
---
id: tb-8a4c
title: Address autocomplete at checkout
owner: priya
branch: '*/tb-8a4c-*'
epic: checkout
priority: 2
type: story
points: 5
---

## Goal

Typing an address on a phone is where mobile checkout goes to die. Offer
completions from the first few characters, and let the customer correct
the result by hand afterwards.

## Acceptance

- [ ] Three characters of a postcode offer the matching addresses
- [ ] Every autocompleted field stays editable
- [ ] The form works unchanged when the lookup is unavailable
EOF

spec tb-e2b1-coupon-rate-limit <<'EOF'
---
id: tb-e2b1
title: Rate-limit the coupon endpoint
owner: sam
branch: '*/tb-e2b1-*'
epic: checkout
priority: 3
type: task
---

## Goal

The coupon endpoint answers valid-or-not, unauthenticated and unlimited,
which makes enumerating every code we have ever issued an afternoon's
work for anybody who notices.

## Acceptance

- [ ] Failed attempts are rate-limited per address and per session
- [ ] A blocked caller cannot tell a wrong code from a rate limit
EOF

git add -A
commit 21 "The backlog for s12 and s13

Every story here is a promise. None of them is a status: filing one is
not delivering it, so these land on main and stay in the backlog."

# ---------------------------------------------------------------------------
# Delivered: work that landed on main. `done` is the merge, nothing else.
# ---------------------------------------------------------------------------

git checkout -q -b feature/tb-3e92-payments-sdk-v4
cat >src/payments/gateway.js <<'EOF'
import { v4 } from "@acme/payments";

export async function charge(amount, method) {
  return v4.charge({ amount, method });
}
EOF
git add -A
commit 17 "Move every call site to the v4 payments SDK

Spec: tb-3e92"
git checkout -q main
GIT_AUTHOR_DATE="$(days_ago 17)" GIT_COMMITTER_DATE="$(days_ago 17)" \
	git merge -q --no-ff -m "Merge branch 'feature/tb-3e92-payments-sdk-v4'

Spec: tb-3e92" feature/tb-3e92-payments-sdk-v4
git branch -q -d feature/tb-3e92-payments-sdk-v4

git checkout -q -b feature/tb-4f2a-one-page-checkout
cat >src/checkout/onepage.js <<'EOF'
import { total } from "./basket.js";

export function render(basket, address, delivery, payment) {
  return { total: total(basket), address, delivery, payment, steps: 1 };
}
EOF
git add -A
commit 14 "Collapse checkout onto one page

Address, delivery and payment stay on screen together, with the basket
total beside them, and every field validates as it is filled.

Spec: tb-4f2a"
cat >src/checkout/validate.js <<'EOF'
export function inline(field, value) {
  return { field, ok: Boolean(value), when: "as-typed" };
}
EOF
git add -A
commit 12 "Validate inline instead of on submit

Spec: tb-4f2a"
git checkout -q main
GIT_AUTHOR_DATE="$(days_ago 11)" GIT_COMMITTER_DATE="$(days_ago 11)" \
	git merge -q --no-ff -m "Merge branch 'feature/tb-4f2a-one-page-checkout'

Spec: tb-4f2a" feature/tb-4f2a-one-page-checkout
git branch -q -d feature/tb-4f2a-one-page-checkout

# Landed, and nobody ever read the promise back: this story derives done and
# is reported as unverified acceptance everywhere, which is the point of it.
git checkout -q -b fix/tb-2b9e-declined-card-basket
cat >src/checkout/session.js <<'EOF'
export function keepBasket(session, basket) {
  session.basket = basket;
  return session;
}
EOF
git add -A
commit 4 "Keep the basket across a declined payment

Spec: tb-2b9e"
git checkout -q main
GIT_AUTHOR_DATE="$(days_ago 3)" GIT_COMMITTER_DATE="$(days_ago 3)" \
	git merge -q --no-ff -m "Merge branch 'fix/tb-2b9e-declined-card-basket'

Spec: tb-2b9e" fix/tb-2b9e-declined-card-basket

# ---------------------------------------------------------------------------
# Regressed: it landed, then it came undone. A revert is a fact about the
# repository, so the board reports it without being told.
# ---------------------------------------------------------------------------

git checkout -q -b fix/tb-1d38-confirmation-total
cat >src/checkout/confirmation.js <<'EOF'
export function summary(order) {
  return { charged: order.chargedAmount, discount: order.discount };
}
EOF
git add -A
commit 9 "Show the amount actually charged on the confirmation page

Spec: tb-1d38"
git checkout -q main
GIT_AUTHOR_DATE="$(days_ago 8)" GIT_COMMITTER_DATE="$(days_ago 8)" \
	git merge -q --no-ff -m "Merge branch 'fix/tb-1d38-confirmation-total'

Spec: tb-1d38" fix/tb-1d38-confirmation-total
git branch -q -d fix/tb-1d38-confirmation-total
GIT_AUTHOR_DATE="$(days_ago 2)" GIT_COMMITTER_DATE="$(days_ago 2)" \
	git revert --no-edit --mainline 1 HEAD >/dev/null

# ---------------------------------------------------------------------------
# In flight: a branch with commits and no merge. Nothing declares this — the
# branch is the declaration.
# ---------------------------------------------------------------------------

git checkout -q -b feature/tb-7c31-apple-pay
cat >src/payments/applepay.js <<'EOF'
export function available(browser) {
  return Boolean(browser.ApplePaySession?.canMakePayments());
}
EOF
git add -A
commit 3 "Show the Apple Pay button only where it can work

Spec: tb-7c31"
cat >src/payments/merchant.js <<'EOF'
export async function validate(session) {
  return fetch("/api/applepay/merchant", { method: "POST", body: session });
}
EOF
git add -A
commit 0 "Validate the merchant session server-side

The certificate never reaches the browser.

Spec: tb-7c31"

# Stalled: the same shape of history, stopped. Nobody set this either; it is
# what a branch looks like when it has not been touched for a fortnight.
git checkout -q main
git checkout -q -b feature/tb-9a15-guest-checkout
cat >src/checkout/guest.js <<'EOF'
export function guestOrder(email, basket) {
  return { email, basket, account: null };
}
EOF
git add -A
commit 16 "Start guest checkout: an order needs an email, not a password

Spec: tb-9a15"

git checkout -q main

printf '\nbuilt %s\n\n' "$TARGET"
printf 'the board this produces:\n'
printf '  truthboard audit %s\n' "$TARGET"
printf '  cd %s && truthboard ui\n\n' "$TARGET"
