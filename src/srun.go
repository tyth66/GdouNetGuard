package campus

import (
	"time"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const srunBase64Alphabet = "LVoJPiCN2R8G90yg+hmFHuacZ1OWMnrsSTXkYpUq/3dlbfKwv6xztjI7DeBE45QA"

const portalAgreeType = "1" // 1 = normal user (portal protocol agreement)

// ---- response types ----

type userInfoResponse struct {
	Error    string `json:"error"`
	Res      string `json:"res"`
	ECode    any    `json:"ecode"`
	ErrorMsg string `json:"error_msg"`
	OnlineIP string `json:"online_ip"`
	ClientIP string `json:"client_ip"`
	UserName string `json:"user_name"`
}

type challengeResponse struct {
	Challenge string `json:"challenge"`
	ClientIP  string `json:"client_ip"`
	Error     string `json:"error"`
	ErrorMsg  string `json:"error_msg"`
	Res       string `json:"res"`
}

type loginResponse struct {
	Error    string `json:"error"`
	Res      string `json:"res"`
	ECode    any    `json:"ecode"`
	ErrorMsg string `json:"error_msg"`
	SucMsg   string `json:"suc_msg"`
}

type logoutResponse struct {
	Error    string `json:"error"`
	Res      string `json:"res"`
	ECode    int    `json:"ecode"`
	ErrorMsg string `json:"error_msg"`
	ClientIP string `json:"client_ip"`
	OnlineIP string `json:"online_ip"`
}

type encodedUserInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IP       string `json:"ip"`
	Acid     string `json:"acid"`
	EncVer   string `json:"enc_ver"`
}

type protocolResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    *protocolData    `json:"data"`
}

type protocolData struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	SerialNumber string `json:"serial_number"`
	Content      string `json:"content"`
}

type agreeResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// portalClient wraps HTTP configuration shared across portal requests.
type portalClient struct {
	baseURL string
	http    *http.Client
}

// newPortalClient creates a portal client with the default SRUN redirect policy.
func newPortalClient(baseURL string, timeout time.Duration) *portalClient {
	return &portalClient{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ---- HTTP portal methods ----

func (c *portalClient) campusOnline(ctx context.Context) (bool, userInfoResponse, error) {
	var info userInfoResponse
	endpoint := mustJoinURL(c.baseURL, "/cgi-bin/rad_user_info")
	body, err := c.getJSONP(ctx, endpoint, url.Values{"callback": {DefaultCallback}})
	if err != nil {
		return false, info, err
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return false, info, err
	}
	return responseOK(info.Error, info.Res), info, nil
}

func (c *portalClient) internetReachable(ctx context.Context, probeURL, probeContains string) bool {
	if probeURL == "" {
		return true
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false
	}
	if probeContains == "" {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return err == nil && strings.Contains(string(body), probeContains)
}

func (c *portalClient) detectClientIP(ctx context.Context, acID, username string) (string, error) {
	portalURL := mustJoinURL(c.baseURL, "/srun_portal_pc")
	u, err := url.Parse(portalURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("ac_id", acID)
	q.Set("theme", "pro")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("portal page returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`ip\s*:\s*"([^"]+)"`)
	matches := re.FindSubmatch(body)
	if len(matches) >= 2 {
		return string(matches[1]), nil
	}

	challenge, err := c.getChallenge(ctx, username, "", acID)
	if err != nil {
		return "", err
	}
	return challenge.ClientIP, nil
}

func (c *portalClient) getChallenge(ctx context.Context, username, ip, acID string) (challengeResponse, error) {
	endpoint := mustJoinURL(c.baseURL, "/cgi-bin/get_challenge")
	values := url.Values{
		"callback": {DefaultCallback},
		"username": {username},
	}
	if ip != "" {
		values.Set("ip", ip)
	}
	body, err := c.getJSONP(ctx, endpoint, values)
	if err != nil {
		return challengeResponse{}, err
	}
	var challenge challengeResponse
	if err := json.Unmarshal(body, &challenge); err != nil {
		return challengeResponse{}, err
	}
	if challenge.Challenge == "" {
		return challenge, fmt.Errorf("challenge missing: res=%s error=%s message=%s", challenge.Res, challenge.Error, challenge.ErrorMsg)
	}
	return challenge, nil
}

// agreePortalProtocol fetches the latest portal agreement and agrees to it
// on behalf of the given user. This is required when UserAgreeSwitch is enabled.
func (c *portalClient) agreePortalProtocol(ctx context.Context, username string) error {
	// 1. Fetch latest protocol
	protoURL := mustJoinURL(c.baseURL, "/v1/srun_portal_agree_new")
	u, err := url.Parse(protoURL)
	if err != nil {
		return fmt.Errorf("protocol URL parse: %w", err)
	}
	q := u.Query()
	q.Set("agree_type", portalAgreeType)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("protocol request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GdouNetGuard/1.3.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("protocol fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("protocol fetch returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("protocol read: %w", err)
	}
	var proto protocolResponse
	if err := json.Unmarshal(body, &proto); err != nil {
		return fmt.Errorf("protocol parse: %w", err)
	}
	if proto.Code != 0 || proto.Data == nil {
		return fmt.Errorf("protocol unavailable: code=%d message=%s", proto.Code, proto.Message)
	}

	// 2. Agree to protocol
	agreeURL := mustJoinURL(c.baseURL, "/v1/srun_portal_agree_bind")
	agreeBody := fmt.Sprintf("agree_id=%d&user_name=%s", proto.Data.ID, url.QueryEscape(username))
	agreeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, agreeURL, strings.NewReader(agreeBody))
	if err != nil {
		return fmt.Errorf("agree request: %w", err)
	}
	agreeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	agreeReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GdouNetGuard/1.3.0")
	agreeResp, err := c.http.Do(agreeReq)
	if err != nil {
		return fmt.Errorf("agree submit: %w", err)
	}
	defer agreeResp.Body.Close()
	if agreeResp.StatusCode < 200 || agreeResp.StatusCode >= 400 {
		return fmt.Errorf("agree submit returned HTTP %d", agreeResp.StatusCode)
	}
	agreeBodyBytes, err := io.ReadAll(io.LimitReader(agreeResp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("agree read: %w", err)
	}
	var agree agreeResponse
	if err := json.Unmarshal(agreeBodyBytes, &agree); err != nil {
		return fmt.Errorf("agree parse: %w", err)
	}
	if agree.Code != 0 {
		return fmt.Errorf("agree rejected: code=%d message=%s", agree.Code, agree.Message)
	}
	return nil
}

func (c *portalClient) portalLogin(ctx context.Context, username, password, ip, acID string) (loginResponse, error) {
	challenge, err := c.getChallenge(ctx, username, ip, acID)
	if err != nil {
		return loginResponse{}, err
	}

	params, err := BuildLoginParams(username, password, ip, acID, challenge.Challenge)
	if err != nil {
		return loginResponse{}, err
	}
	params.Set("callback", DefaultCallback)

	body, err := c.getJSONP(ctx, mustJoinURL(c.baseURL, "/cgi-bin/srun_portal"), params)
	if err != nil {
		return loginResponse{}, err
	}
	var resp loginResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return loginResponse{}, err
	}
	return resp, nil
}

func (c *portalClient) portalLogout(ctx context.Context, username, ip, acID string) error {
	endpoint := mustJoinURL(c.baseURL, "/cgi-bin/srun_portal")
	params := url.Values{
		"callback": {DefaultCallback},
		"action":   {"logout"},
		"ac_id":    {acID},
		"username": {username},
		"ip":       {ip},
	}
	body, err := c.getJSONP(ctx, endpoint, params)
	if err != nil {
		return err
	}
	var resp logoutResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse logout response: %w", err)
	}
	if !responseOK(resp.Error, resp.Res) {
		return fmt.Errorf("logout rejected: res=%s error=%s message=%s", resp.Res, resp.Error, resp.ErrorMsg)
	}
	return nil
}

func (c *portalClient) getJSONP(ctx context.Context, endpoint string, values url.Values) ([]byte, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for key, items := range values {
		for _, item := range items {
			q.Add(key, item)
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GdouNetGuard/1.3.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s returned HTTP %d", endpoint, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return UnwrapJSONP(body)
}

// ---- protocol crypto ----

// BuildLoginParams builds the SRUN login form parameters.
func BuildLoginParams(username, password, ip, acid, token string) (url.Values, error) {
	const (
		encVer = "srun_bx1"
		n      = "200"
		typ    = "1"
	)

	hmd5 := hmacMD5Hex(token, password)
	info, err := encodeUserInfo(encodedUserInfo{
		Username: username,
		Password: password,
		IP:       ip,
		Acid:     acid,
		EncVer:   encVer,
	}, token)
	if err != nil {
		return nil, err
	}

	sumInput := token + username +
		token + hmd5 +
		token + acid +
		token + ip +
		token + n +
		token + typ +
		token + info

	return url.Values{
		"action":       {"login"},
		"username":     {username},
		"password":     {"{MD5}" + hmd5},
		"os":           {"Windows NT"},
		"name":         {"Windows"},
		"double_stack": {"0"},
		"chksum":       {sha1Hex(sumInput)},
		"info":         {info},
		"ac_id":        {acid},
		"ip":           {ip},
		"n":            {n},
		"type":         {typ},
	}, nil
}

func encodeUserInfo(info encodedUserInfo, token string) (string, error) {
	payload, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	encoded := xEncode(string(payload), token)
	return "{SRBX1}" + base64.NewEncoding(srunBase64Alphabet).EncodeToString(encoded), nil
}

func xEncode(value, key string) []byte {
	if value == "" {
		return nil
	}
	v := stringToUint32(value, true)
	k := stringToUint32(key, false)
	for len(k) < 4 {
		k = append(k, 0)
	}

	n := len(v) - 1
	z := v[n]
	y := v[0]
	c := uint32(0x86014019 | 0x183639A0)
	q := 6 + 52/(n+1)
	var d uint32

	for ; q > 0; q-- {
		d = d + c
		e := (d >> 2) & 3
		var p int
		for p = 0; p < n; p++ {
			y = v[p+1]
			m := (z >> 5) ^ (y << 2)
			m += (y >> 3) ^ (z << 4) ^ (d ^ y)
			m += k[(p&3)^int(e)] ^ z
			v[p] += m
			z = v[p]
		}

		y = v[0]
		m := (z >> 5) ^ (y << 2)
		m += (y >> 3) ^ (z << 4) ^ (d ^ y)
		m += k[(p&3)^int(e)] ^ z
		v[n] += m
		z = v[n]
	}

	return uint32ToBytes(v)
}

func stringToUint32(s string, includeLength bool) []uint32 {
	raw := []byte(s)
	out := make([]uint32, (len(raw)+3)/4)
	for i := 0; i < len(raw); i += 4 {
		var value uint32
		for shift := 0; shift < 4; shift++ {
			idx := i + shift
			if idx < len(raw) {
				value |= uint32(raw[idx]) << (8 * shift)
			}
		}
		out[i>>2] = value
	}
	if includeLength {
		out = append(out, uint32(len(raw)))
	}
	return out
}

func uint32ToBytes(values []uint32) []byte {
	out := make([]byte, 0, len(values)*4)
	for _, value := range values {
		out = append(out,
			byte(value&0xff),
			byte((value>>8)&0xff),
			byte((value>>16)&0xff),
			byte((value>>24)&0xff),
		)
	}
	return out
}

func hmacMD5Hex(key, message string) string {
	mac := hmac.New(md5.New, []byte(key))
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func sha1Hex(message string) string {
	sum := sha1.Sum([]byte(message))
	return hex.EncodeToString(sum[:])
}

// UnwrapJSONP strips the JSONP callback wrapper from a response body.
func UnwrapJSONP(body []byte) ([]byte, error) {
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "{") {
		return []byte(text), nil
	}
	open := strings.IndexByte(text, '(')
	close := strings.LastIndexByte(text, ')')
	if open < 0 || close <= open {
		return nil, fmt.Errorf("response is not JSON or JSONP: %.120s", text)
	}
	return []byte(text[open+1 : close]), nil
}

// ---- helpers ----

func mustJoinURL(baseURL, path string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + path
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func responseOK(errorValue, resValue string) bool {
	return strings.EqualFold(errorValue, "ok") || strings.EqualFold(resValue, "ok")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
