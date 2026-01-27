#!/bin/bash

# Rate Limiter Stress Test Suite
# Tests global rate limiting (token bucket) and per-client limiting (sliding window)

PORT=8090
BASE_URL="http://localhost:$PORT"

# Start server
go run . &
PID=$!
sleep 3
SERVER_PID=$(lsof -t -i :$PORT)

echo "=== Rate Limiter Stress Test Suite ==="
echo ""

# ============================================
# Test 1: Index route (global limit only)
# ============================================
echo "--- Test 1: Index route (global limit only) ---"
hey -n 100000 -c 30 -t 5 "$BASE_URL/" 2>/dev/null | grep -E "responses|Status"
echo ""

# ============================================
# Test 2: Missing API key (should return 401)
# ============================================
echo "--- Test 2: Missing API key on protected route ---"
curl -s -w "Status: %{http_code}\n" "$BASE_URL/s/testshort"
echo ""

# ============================================
# Test 3: POST /shorten - create a short URL to example.com
# ============================================
echo "--- Test 3: Create short URL to example.com ---"
SHORTEN_RESPONSE=$(curl -s -X POST \
  -H "X-API-Key: testclient1" \
  -H "Content-Type: application/json" \
  -d '{"original":"https://example.com"}' \
  "$BASE_URL/shorten")
echo "Response: $SHORTEN_RESPONSE"

# Extract short URL code from response (assumes format "Short URL: XXXXXXXXXX")
SHORT_CODE=$(echo "$SHORTEN_RESPONSE" | grep -oE '[A-Za-z0-9]{10}' | head -1)
echo "Extracted short code: $SHORT_CODE"
echo ""

# ============================================
# Test 4: GET /{shortUrl} - retrieve and redirect to example.com
# ============================================
echo "--- Test 4: Retrieve short URL (should redirect to example.com ) ---"
curl -s -w "Status: %{http_code}\n" -H "X-API-Key: testclient1" -L "$BASE_URL/s/$SHORT_CODE" | head -5
echo ""

# ============================================
# Test 5: Per-client rate limit test
# Exceed per-client limit with rapid requests from same client
# ============================================
echo "--- Test 5: Per-client rate limit (single client burst) ---"
echo "Sending 30 rapid requests from same API key..."
hey -n 30 -c 5 -t 5 \
  -H "X-API-Key: burstclient" \
  -m POST \
  -H "Content-Type: application/json" \
  -d '{"original":"https://example.com/burst-test"}' \
  "$BASE_URL/shorten" 2>/dev/null | grep -E "responses|Status"
echo ""

# ============================================
# Test 6: Multiple clients (each within their limit)
# ============================================
echo "--- Test 6: Multiple clients (sequential, within limits) ---"
for i in {1..5}; do
  echo "Client $i:"
  hey -n 10 -c 2 -t 5 \
    -H "X-API-Key: client$i" \
    -m POST \
    -H "Content-Type: application/json" \
    -d "{\"original\":\"https://example.com/client$i\"}" \
    "$BASE_URL/shorten" 2>/dev/null | grep -E "responses|Status"
done
echo ""

# ============================================
# Test 7: Global rate limit exhaustion
# Many concurrent requests to overwhelm global bucket
# ============================================
echo "--- Test 7: Global rate limit stress test ---"
hey -n 200 -c 20 -t 5 \
  -H "X-API-Key: stressclient" \
  -m POST \
  -H "Content-Type: application/json" \
  -d '{"original":"https://example.com/stress"}' \
  "$BASE_URL/shorten" 2>/dev/null | grep -E "responses|Status"
echo ""

# ============================================
# Test 8: Recovery after exhaustion
# ============================================
echo "--- Test 8: Recovery after rate limit (wait 3s) ---"
sleep 3
hey -n 20 -c 5 -t 5 \
  -H "X-API-Key: recoveryclient" \
  -m POST \
  -H "Content-Type: application/json" \
  -d '{"original":"https://example.com/recovery"}' \
  "$BASE_URL/shorten" 2>/dev/null | grep -E "responses|Status"
echo ""

# ============================================
# Test 9: Invalid URL validation
# ============================================
echo "--- Test 9: Invalid URL (should return 400) ---"
curl -s -w "\nStatus: %{http_code}\n" -X POST \
  -H "X-API-Key: testclient1" \
  -H "Content-Type: application/json" \
  -d '{"original":"not-a-valid-url"}' \
  "$BASE_URL/shorten"
echo ""

echo "--- Test 10: Invalid scheme (should return 400) ---"
curl -s -w "\nStatus: %{http_code}\n" -X POST \
  -H "X-API-Key: testclient1" \
  -H "Content-Type: application/json" \
  -d '{"original":"ftp://example.com/file"}' \
  "$BASE_URL/shorten"
echo ""

# ============================================
# Cleanup
# ============================================
echo "=== Tests Complete ==="
echo "Shutting down server..."
kill $SERVER_PID 2>/dev/null
wait $SERVER_PID 2>/dev/null
kill $PID 2>/dev/null
wait $PID 2>/dev/null
echo "Done."
