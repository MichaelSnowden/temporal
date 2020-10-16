// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cassandra

import (
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"go.temporal.io/server/common/auth"
	"go.temporal.io/server/common/service/config"
)

func TestNewCassandraCluster(t *testing.T) {
	tests := map[string]struct {
		cfg config.Cassandra
		err error
	}{
		"emptyConfig": {
			cfg: config.Cassandra{},
			err: nil,
		},
		"caCert_badBase64": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{Enabled: true, CaData: "this isn't base64"},
			},
			err: base64.CorruptInputError(4),
		},
		"caCert_badPEM": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{Enabled: true, CaData: "dGhpcyBpc24ndCBhIFBFTSBjZXJ0"},
			},
			err: errors.New("failed to load decoded CA Cert as PEM"),
		},
		"caCert_good": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{
					Enabled: true,
					CaData:  "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURvRENDQW9pZ0F3SUJBZ0lVTDFKdUx0K0dRcWNuM0pDZGNiaUxibjBmSjhBd0RRWUpLb1pJaHZjTkFRRUwKQlFBd2FERUxNQWtHQTFVRUJoTUNWVk14RXpBUkJnTlZCQWdUQ2xkaGMyaHBibWQwYjI0eEVEQU9CZ05WQkFjVApCMU5sWVhSMGJHVXhEVEFMQmdOVkJBb1RCRlZ1YVhReERUQUxCZ05WQkFzVEJGUmxjM1F4RkRBU0JnTlZCQU1UCkMxVnVhWFJVWlhOMElFTkJNQjRYRFRJd01Ea3hOekUzTXpVd01Gb1hEVEkxTURreE5qRTNNelV3TUZvd2FERUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENsZGhjMmhwYm1kMGIyNHhFREFPQmdOVkJBY1RCMU5sWVhSMApiR1V4RFRBTEJnTlZCQW9UQkZWdWFYUXhEVEFMQmdOVkJBc1RCRlJsYzNReEZEQVNCZ05WQkFNVEMxVnVhWFJVClpYTjBJRU5CTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUF0ZzU5SGU2MDVlYjIKcThGYUpycHBoRVNPZnJiVEdIQlhXRk41Z0N1QUlZMmVNTUdKSnFwWERrUjNGWko2TFZaYXFkVm9rWmkzeVRIOQprWW5uTEhBRDJJKzd5M0FnczB0WWZucmx0MGhtWjNleVlRSGk0Y1d0Vkd3aVoycW0yQnZMbzJVMENkeXRSSjRRCjNlQVQyeVRrTnZ4Wm9XeUhHK09icjZ4UFByMjh2bWo3Q0txVnNLQ0FIVnlqdXlybXRJcHdkbWVpVTlFbTFTTUgKSVBLR0pJQ29NeGl4NXNDdHVqZmRSTWJTU2hIRFluUmdmMkx2enIxVk5mZkdaS01YekJaekkyZ3BJZm9YaGZVUwpkdmNlUTVoWXo4emdEY2hDOG1laEM3bU12Myt6Q3d6OWtGbmJpYnBvSVdGcStGbzYzeHNnc255dFlQTXY0cmltClgwSWRwZlA2VVFJREFRQUJvMEl3UURBT0JnTlZIUThCQWY4RUJBTUNBUVl3RHdZRFZSMFRBUUgvQkFVd0F3RUIKL3pBZEJnTlZIUTRFRmdRVWUzY0MvMllrTWRmRUZUbUliMW84M1U0VWgxY3dEUVlKS29aSWh2Y05BUUVMQlFBRApnZ0VCQUN2TTVURG9BM0FFRFlrcFlueWwwVlpmRDdKRVhWSEJ5WTEyeG9jUHM4TGJzNEtNS1NtUGVld0dIU25WCisrQVdFdG8vWlFjUnVVcm9SR2ZFRDRTU3kyT0tyNGh4M0J0UmNGRkZrdFg4U2Uwck5rSitaSHVoVFBWdWQ5L00KUXRBenl2UWVkcDBXQlcydDBvWkhDcVNOWmMvSWFYWGNxeTdocHpLOHBLZTNOUXYyUkdHVkEybWRDR1oxUE5rMgpFTXhMVnhoUURkbTNKRWJ4SEJPNCtVWm45MHVDd1BGc25rVFFmNm53WTErMjNMc3lheFkxWXFkeXZHTzhjdEc0CnozUmRTbTVJM25XaTNERFd3TnhuZ1lpMCtBL01VQ0FBYjBOejluSXI3dzB5UlJpWHJha1hUYjlaOC9GWE1JdHEKdG5wckJzK3hhYzhBVWxzcEw3cCtUWmRUMFdNPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
				},
			},
			err: nil,
		},
		"clientCert_badbase64cert": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{
					Enabled:  true,
					CertData: "this ain't base64",
					KeyData:  "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVBellWcmcwdTBscWF2VTJGc1p3dU1nOTl2UDU0akRxRUtUYTNzaWkwcjNsQUFES3RxCk1LejhFVVdHdVlLeVV1ZWRjdGdkbVdyVWpBam1yby8yTXdYbnB6dWVNWnhKKzNmblhrSHNsVFdURUZZL1N1cSsKVXg0UjRuN2JkK09qclJnQjVUSE9GS3V2cVZsckpjZE55WmR2Y3pld29WWTJZeWxIOGZvcFV2RStvaS9kWGh2SgpYYy85MkxlNmNIRWx4Ni9nUnM5VzdzK2NIL3E1QVVnc2VmNFRyR0VNUU5lR0lOUEFTcCtBM1pnMzgxVmZHTG4wCjhiSHRXUHJNNEo5NmhEOFlYZ2dYTDdYM0tBeHB3VTZRV1NpTnAzVUJWTHpFUXJkWDQwWVREais0QTZZU3dCWXUKQTZ6U1ZRTko2RDdEV1pEVDJPakJ5RGtLWGF0OE5CbVowMUkvN3dJREFRQUJBb0lCQUNkK2hpU2EvYjhkbFArZQo3eWYyTGpDQlZXMlNSQVpocUFzNWF3VTZuUDJCdmlDeEtCem1nU0lJakZWRjZtTElJNWVZTkVmeElac3ZjclVFCjhUam8zNVZoZllybkQ4aUZTQzd5MkRYc0w3Q3FBa3V4UkpYUVozdHhDVmZHcFFOMFk1alpzMUtCazZZbGl0T2QKc3pNVUtOU3BWUVlML1RPZEVUaE03SGdGNkJWZWFRWFhSd3FsSS8raGwrNjdKeGV3eDh5RkcrQTIwTDdqMFcrUwp2dnVXOUJJNkVFaVZJUURMVk1TNktsL0FacGw2dENhN05ycTMrM1RldlI2ck56U2tpT1htYmtpdm5lUUZDRWtMCkg5N1NjVUJ4cEx0SVBFZ2dJUFhEVE9GemdaMFBocmpJT2d2WnA1QVZ0ay8rRDdDU05mb3MzSjl4REhaNnJTcXkKQ042eUpiRUNnWUVBOUJLYTJPMEhqL2V0U2Z2RDBXd0NuRnJXUzF0aE9PdWFib3A4Ymc4cGV3RC9pZkEwRDloNwp4N3JTVkRNT3VhK3hRTHQyMWRkOU1TQnlmOCtIS3YvMjdkb1ZWWjJIbkZMYkx0dGJDNHk3SXlxR2o3Rk9kV2JiCkVERVdlMXlNWjN3VlZsbFQweXdmN0ZJS0pDUTYvbzV5VmZMNWRmNmlidk5zQjdVVDlsdWVCUDBDZ1lFQTE1Q0gKVGRNMEJ5dXd1V0lKREpZUEZ3TlVvWnFjNnNyb1VEL1QzUVh0aGpxazBuamJpR0FVTllxOXlPYjVhRVhQVjNSVAplYmpkZk5MUHV4YmNiZTNUWUN1VVdtaU5tSmdSOWZKeHNPaldvTFBySUlPcEJZMUVkSFZuQ0N0VWh4M2h1VVBHCitQeE8xTkgxZXdZU0FSSWs1MjNJeDd4MnNnZEZycTlxdHNOMmdsc0NnWUVBMWxIdi8wUkVPN3MxUTUzOFdVMEwKRGRrR0M2MzJOVkZOam51MHY4QTRvSFpEN2hBcTV5OGxva0QrcUVrZFNSaHFBWG1iNURNUkQ2NTZYSmtUREVNdgp4YlNXdjFOUTNZZzBSM1QvQWFsV09vOEJFZlNUL0t1UStTcmhudm1wb01Wb3h5WXhZV0dCdHJaamlWRDNMTWhRCnhnQlI1YmJ2VTVZVTZyK3JBODEzZU5FQ2dZQmNIOEF6V2xlWjJPb2x6K2ZlSVNOQnlvS1lyZUx4MU5XRHRrTnMKNmVPZ3dkOCtzN2ZlaUhFYWtMaWE2MXNiWFBwSGZjZE85ZHB5UmdYUkJ1d2RiczR2QTNEYlVtTnhHMHhSdDlNdQpyOU5KeXBwcHd0cXhMTFpjcnUvaFplTXgrMnRFS2RzVy9YMFRKc2VxVStYTjMxczJMSXpxRDNrS2pHRVRUcFJrCmx4UkdrUUtCZ1FEUkQveTJvNzloSktaQUJKeDVkdUoyOFl6b3JmTndhT2hHY0RvZlhFeGJqdUNUS003QS9rcy8KOGlvR0lEdC9iUGN3c0cxWDRySHZnMU5XWFlWSFkxVUl2VkFOUUExQ3V2TDhDbytnemRZNkYvcnQzcTZjT2xGbgpnaTA3Q3R3dlJCYWw2U1hCVmZkU3ZhNGY0c0p5OGhRbXRJOWdwd3VIemR5UnMya01Ka1hNQVE9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=",
				},
			},
			err: base64.CorruptInputError(4),
		},
		"clientCert_badbase64key": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{
					Enabled:  true,
					CertData: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVEVENDQXZXZ0F3SUJBZ0lDQm5vd0RRWUpLb1pJaHZjTkFRRUxCUUF3Z2FVeEN6QUpCZ05WQkFZVEFsVlQKTVFzd0NRWURWUVFJRXdKRFFURVVNQklHQTFVRUJ4TUxVMkZ1ZEdFZ1EyeGhjbUV4RVRBUEJnTlZCQW9UQ0VSaApkR0ZUZEdGNE1RNHdEQVlEVlFRTEV3VkRiRzkxWkRGUU1FNEdBMVVFQXhOSFkyRXVNMk5oTm1ZeE9XRXRabUZsCk9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1a1lpNWhjM1J5WVM1a1lYUmgKYzNSaGVDNWpiMjB3SGhjTk1qQXdPVE13TVRjeU56RXpXaGNOTXpBd09UTXdNVGN5TnpFeldqQ0JxVEVMTUFrRwpBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CTVJRd0VnWURWUVFIRXd0VFlXNTBZU0JEYkdGeVlURVJNQThHCkExVUVDaE1JUkdGMFlWTjBZWGd4RGpBTUJnTlZCQXNUQlVOc2IzVmtNVlF3VWdZRFZRUURFMHRqYkdsbGJuUXUKTTJOaE5tWXhPV0V0Wm1GbE9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1awpZaTVoYzNSeVlTNWtZWFJoYzNSaGVDNWpiMjB3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQkR3QXdnZ0VLCkFvSUJBUUROaFd1RFM3U1dwcTlUWVd4bkM0eUQzMjgvbmlNT29RcE5yZXlLTFN2ZVVBQU1xMm93clB3UlJZYTUKZ3JKUzU1MXkyQjJaYXRTTUNPYXVqL1l6QmVlbk81NHhuRW43ZCtkZVFleVZOWk1RVmo5SzZyNVRIaEhpZnR0Mwo0Nk90R0FIbE1jNFVxNitwV1dzbHgwM0psMjl6TjdDaFZqWmpLVWZ4K2lsUzhUNmlMOTFlRzhsZHovM1l0N3B3CmNTWEhyK0JHejFidXo1d2YrcmtCU0N4NS9oT3NZUXhBMTRZZzA4QktuNERkbURmelZWOFl1ZlR4c2UxWStzemcKbjNxRVB4aGVDQmN2dGZjb0RHbkJUcEJaS0kybmRRRlV2TVJDdDFmalJoTU9QN2dEcGhMQUZpNERyTkpWQTBubwpQc05aa05QWTZNSElPUXBkcTN3MEdablRVai92QWdNQkFBR2pRVEEvTUE0R0ExVWREd0VCL3dRRUF3SUhnREFkCkJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEZ1lEVlIwT0JBY0VCUUVDQXdRR01BMEcKQ1NxR1NJYjNEUUVCQ3dVQUE0SUJBUUNxOWoyRXZYZFFzck9jTVlheUtWL0M0dGw2WTNFOEJCSU1rMHpSZ0tmWgpZVFV2V0RSeFVLY0hDeXFnT1lWWjhQbzlFaVpmL2lFTEEzbEtmd3VwNVZTbVRCODdvUVNTTEx3ZmJ1YnlYcU56ClJzSDdBSlNKZ0drMzJ4cWdpYmZzVU81MzA2SUVtTkRLK3ZkUTZjSk51bUtaeGhvRkZqTHY3QVI2RXBsVDRpKzcKSEprQk5XQnJFejNyaVhNL0VFSnN5V0p4dWJBL3pkcUk4WkI5ZFNJcmZ3NWp5N3lGNWw4ZjNNWjBjNnJzVDluZQpRTXVIeFRBNC95UnIrenlGZ3oyNDlwTHoybHlJT01OTmlxVkNubzVER1ZNSHQ0T08zbnVyT2lIelJUSnVsbDFKCkxvaU1xK2FLVFFITUU4T1ZKRUhvbHgrT242Q3JvSHRLa1Y4SER3WCtsb2syCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
					KeyData:  "this ain't base64",
				},
			},
			err: base64.CorruptInputError(4),
		},
		"clientCert_missingprivatekey": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{
					Enabled:  true,
					CertData: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVEVENDQXZXZ0F3SUJBZ0lDQm5vd0RRWUpLb1pJaHZjTkFRRUxCUUF3Z2FVeEN6QUpCZ05WQkFZVEFsVlQKTVFzd0NRWURWUVFJRXdKRFFURVVNQklHQTFVRUJ4TUxVMkZ1ZEdFZ1EyeGhjbUV4RVRBUEJnTlZCQW9UQ0VSaApkR0ZUZEdGNE1RNHdEQVlEVlFRTEV3VkRiRzkxWkRGUU1FNEdBMVVFQXhOSFkyRXVNMk5oTm1ZeE9XRXRabUZsCk9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1a1lpNWhjM1J5WVM1a1lYUmgKYzNSaGVDNWpiMjB3SGhjTk1qQXdPVE13TVRjeU56RXpXaGNOTXpBd09UTXdNVGN5TnpFeldqQ0JxVEVMTUFrRwpBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CTVJRd0VnWURWUVFIRXd0VFlXNTBZU0JEYkdGeVlURVJNQThHCkExVUVDaE1JUkdGMFlWTjBZWGd4RGpBTUJnTlZCQXNUQlVOc2IzVmtNVlF3VWdZRFZRUURFMHRqYkdsbGJuUXUKTTJOaE5tWXhPV0V0Wm1GbE9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1awpZaTVoYzNSeVlTNWtZWFJoYzNSaGVDNWpiMjB3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQkR3QXdnZ0VLCkFvSUJBUUROaFd1RFM3U1dwcTlUWVd4bkM0eUQzMjgvbmlNT29RcE5yZXlLTFN2ZVVBQU1xMm93clB3UlJZYTUKZ3JKUzU1MXkyQjJaYXRTTUNPYXVqL1l6QmVlbk81NHhuRW43ZCtkZVFleVZOWk1RVmo5SzZyNVRIaEhpZnR0Mwo0Nk90R0FIbE1jNFVxNitwV1dzbHgwM0psMjl6TjdDaFZqWmpLVWZ4K2lsUzhUNmlMOTFlRzhsZHovM1l0N3B3CmNTWEhyK0JHejFidXo1d2YrcmtCU0N4NS9oT3NZUXhBMTRZZzA4QktuNERkbURmelZWOFl1ZlR4c2UxWStzemcKbjNxRVB4aGVDQmN2dGZjb0RHbkJUcEJaS0kybmRRRlV2TVJDdDFmalJoTU9QN2dEcGhMQUZpNERyTkpWQTBubwpQc05aa05QWTZNSElPUXBkcTN3MEdablRVai92QWdNQkFBR2pRVEEvTUE0R0ExVWREd0VCL3dRRUF3SUhnREFkCkJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEZ1lEVlIwT0JBY0VCUUVDQXdRR01BMEcKQ1NxR1NJYjNEUUVCQ3dVQUE0SUJBUUNxOWoyRXZYZFFzck9jTVlheUtWL0M0dGw2WTNFOEJCSU1rMHpSZ0tmWgpZVFV2V0RSeFVLY0hDeXFnT1lWWjhQbzlFaVpmL2lFTEEzbEtmd3VwNVZTbVRCODdvUVNTTEx3ZmJ1YnlYcU56ClJzSDdBSlNKZ0drMzJ4cWdpYmZzVU81MzA2SUVtTkRLK3ZkUTZjSk51bUtaeGhvRkZqTHY3QVI2RXBsVDRpKzcKSEprQk5XQnJFejNyaVhNL0VFSnN5V0p4dWJBL3pkcUk4WkI5ZFNJcmZ3NWp5N3lGNWw4ZjNNWjBjNnJzVDluZQpRTXVIeFRBNC95UnIrenlGZ3oyNDlwTHoybHlJT01OTmlxVkNubzVER1ZNSHQ0T08zbnVyT2lIelJUSnVsbDFKCkxvaU1xK2FLVFFITUU4T1ZKRUhvbHgrT242Q3JvSHRLa1Y4SER3WCtsb2syCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
					KeyData:  "",
				},
			},
			err: fmt.Errorf("unable to generate x509 key pair: %w", errors.New("tls: failed to find any PEM data in key input")),
		},
		"clientCert_duplicate_cert": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{
					Enabled:  true,
					CertData: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVEVENDQXZXZ0F3SUJBZ0lDQm5vd0RRWUpLb1pJaHZjTkFRRUxCUUF3Z2FVeEN6QUpCZ05WQkFZVEFsVlQKTVFzd0NRWURWUVFJRXdKRFFURVVNQklHQTFVRUJ4TUxVMkZ1ZEdFZ1EyeGhjbUV4RVRBUEJnTlZCQW9UQ0VSaApkR0ZUZEdGNE1RNHdEQVlEVlFRTEV3VkRiRzkxWkRGUU1FNEdBMVVFQXhOSFkyRXVNMk5oTm1ZeE9XRXRabUZsCk9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1a1lpNWhjM1J5WVM1a1lYUmgKYzNSaGVDNWpiMjB3SGhjTk1qQXdPVE13TVRjeU56RXpXaGNOTXpBd09UTXdNVGN5TnpFeldqQ0JxVEVMTUFrRwpBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CTVJRd0VnWURWUVFIRXd0VFlXNTBZU0JEYkdGeVlURVJNQThHCkExVUVDaE1JUkdGMFlWTjBZWGd4RGpBTUJnTlZCQXNUQlVOc2IzVmtNVlF3VWdZRFZRUURFMHRqYkdsbGJuUXUKTTJOaE5tWXhPV0V0Wm1GbE9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1awpZaTVoYzNSeVlTNWtZWFJoYzNSaGVDNWpiMjB3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQkR3QXdnZ0VLCkFvSUJBUUROaFd1RFM3U1dwcTlUWVd4bkM0eUQzMjgvbmlNT29RcE5yZXlLTFN2ZVVBQU1xMm93clB3UlJZYTUKZ3JKUzU1MXkyQjJaYXRTTUNPYXVqL1l6QmVlbk81NHhuRW43ZCtkZVFleVZOWk1RVmo5SzZyNVRIaEhpZnR0Mwo0Nk90R0FIbE1jNFVxNitwV1dzbHgwM0psMjl6TjdDaFZqWmpLVWZ4K2lsUzhUNmlMOTFlRzhsZHovM1l0N3B3CmNTWEhyK0JHejFidXo1d2YrcmtCU0N4NS9oT3NZUXhBMTRZZzA4QktuNERkbURmelZWOFl1ZlR4c2UxWStzemcKbjNxRVB4aGVDQmN2dGZjb0RHbkJUcEJaS0kybmRRRlV2TVJDdDFmalJoTU9QN2dEcGhMQUZpNERyTkpWQTBubwpQc05aa05QWTZNSElPUXBkcTN3MEdablRVai92QWdNQkFBR2pRVEEvTUE0R0ExVWREd0VCL3dRRUF3SUhnREFkCkJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEZ1lEVlIwT0JBY0VCUUVDQXdRR01BMEcKQ1NxR1NJYjNEUUVCQ3dVQUE0SUJBUUNxOWoyRXZYZFFzck9jTVlheUtWL0M0dGw2WTNFOEJCSU1rMHpSZ0tmWgpZVFV2V0RSeFVLY0hDeXFnT1lWWjhQbzlFaVpmL2lFTEEzbEtmd3VwNVZTbVRCODdvUVNTTEx3ZmJ1YnlYcU56ClJzSDdBSlNKZ0drMzJ4cWdpYmZzVU81MzA2SUVtTkRLK3ZkUTZjSk51bUtaeGhvRkZqTHY3QVI2RXBsVDRpKzcKSEprQk5XQnJFejNyaVhNL0VFSnN5V0p4dWJBL3pkcUk4WkI5ZFNJcmZ3NWp5N3lGNWw4ZjNNWjBjNnJzVDluZQpRTXVIeFRBNC95UnIrenlGZ3oyNDlwTHoybHlJT01OTmlxVkNubzVER1ZNSHQ0T08zbnVyT2lIelJUSnVsbDFKCkxvaU1xK2FLVFFITUU4T1ZKRUhvbHgrT242Q3JvSHRLa1Y4SER3WCtsb2syCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
					CertFile: "/a/b/c",
				},
			},
			err: errors.New("Cannot specify both certData and certFile properties"),
		},
		"clientCert_duplicate_key": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{
					Enabled: true,
					KeyData: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVEVENDQXZXZ0F3SUJBZ0lDQm5vd0RRWUpLb1pJaHZjTkFRRUxCUUF3Z2FVeEN6QUpCZ05WQkFZVEFsVlQKTVFzd0NRWURWUVFJRXdKRFFURVVNQklHQTFVRUJ4TUxVMkZ1ZEdFZ1EyeGhjbUV4RVRBUEJnTlZCQW9UQ0VSaApkR0ZUZEdGNE1RNHdEQVlEVlFRTEV3VkRiRzkxWkRGUU1FNEdBMVVFQXhOSFkyRXVNMk5oTm1ZeE9XRXRabUZsCk9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1a1lpNWhjM1J5WVM1a1lYUmgKYzNSaGVDNWpiMjB3SGhjTk1qQXdPVE13TVRjeU56RXpXaGNOTXpBd09UTXdNVGN5TnpFeldqQ0JxVEVMTUFrRwpBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CTVJRd0VnWURWUVFIRXd0VFlXNTBZU0JEYkdGeVlURVJNQThHCkExVUVDaE1JUkdGMFlWTjBZWGd4RGpBTUJnTlZCQXNUQlVOc2IzVmtNVlF3VWdZRFZRUURFMHRqYkdsbGJuUXUKTTJOaE5tWXhPV0V0Wm1GbE9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1awpZaTVoYzNSeVlTNWtZWFJoYzNSaGVDNWpiMjB3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQkR3QXdnZ0VLCkFvSUJBUUROaFd1RFM3U1dwcTlUWVd4bkM0eUQzMjgvbmlNT29RcE5yZXlLTFN2ZVVBQU1xMm93clB3UlJZYTUKZ3JKUzU1MXkyQjJaYXRTTUNPYXVqL1l6QmVlbk81NHhuRW43ZCtkZVFleVZOWk1RVmo5SzZyNVRIaEhpZnR0Mwo0Nk90R0FIbE1jNFVxNitwV1dzbHgwM0psMjl6TjdDaFZqWmpLVWZ4K2lsUzhUNmlMOTFlRzhsZHovM1l0N3B3CmNTWEhyK0JHejFidXo1d2YrcmtCU0N4NS9oT3NZUXhBMTRZZzA4QktuNERkbURmelZWOFl1ZlR4c2UxWStzemcKbjNxRVB4aGVDQmN2dGZjb0RHbkJUcEJaS0kybmRRRlV2TVJDdDFmalJoTU9QN2dEcGhMQUZpNERyTkpWQTBubwpQc05aa05QWTZNSElPUXBkcTN3MEdablRVai92QWdNQkFBR2pRVEEvTUE0R0ExVWREd0VCL3dRRUF3SUhnREFkCkJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEZ1lEVlIwT0JBY0VCUUVDQXdRR01BMEcKQ1NxR1NJYjNEUUVCQ3dVQUE0SUJBUUNxOWoyRXZYZFFzck9jTVlheUtWL0M0dGw2WTNFOEJCSU1rMHpSZ0tmWgpZVFV2V0RSeFVLY0hDeXFnT1lWWjhQbzlFaVpmL2lFTEEzbEtmd3VwNVZTbVRCODdvUVNTTEx3ZmJ1YnlYcU56ClJzSDdBSlNKZ0drMzJ4cWdpYmZzVU81MzA2SUVtTkRLK3ZkUTZjSk51bUtaeGhvRkZqTHY3QVI2RXBsVDRpKzcKSEprQk5XQnJFejNyaVhNL0VFSnN5V0p4dWJBL3pkcUk4WkI5ZFNJcmZ3NWp5N3lGNWw4ZjNNWjBjNnJzVDluZQpRTXVIeFRBNC95UnIrenlGZ3oyNDlwTHoybHlJT01OTmlxVkNubzVER1ZNSHQ0T08zbnVyT2lIelJUSnVsbDFKCkxvaU1xK2FLVFFITUU4T1ZKRUhvbHgrT242Q3JvSHRLa1Y4SER3WCtsb2syCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
					KeyFile: "/a/b/c",
				},
			},
			err: errors.New("Cannot specify both keyData and keyFile properties"),
		},
		"clientCert_duplicate_ca": {
			cfg: config.Cassandra{
				TLS: &auth.TLS{
					Enabled: true,
					CaData:  "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUVEVENDQXZXZ0F3SUJBZ0lDQm5vd0RRWUpLb1pJaHZjTkFRRUxCUUF3Z2FVeEN6QUpCZ05WQkFZVEFsVlQKTVFzd0NRWURWUVFJRXdKRFFURVVNQklHQTFVRUJ4TUxVMkZ1ZEdFZ1EyeGhjbUV4RVRBUEJnTlZCQW9UQ0VSaApkR0ZUZEdGNE1RNHdEQVlEVlFRTEV3VkRiRzkxWkRGUU1FNEdBMVVFQXhOSFkyRXVNMk5oTm1ZeE9XRXRabUZsCk9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1a1lpNWhjM1J5WVM1a1lYUmgKYzNSaGVDNWpiMjB3SGhjTk1qQXdPVE13TVRjeU56RXpXaGNOTXpBd09UTXdNVGN5TnpFeldqQ0JxVEVMTUFrRwpBMVVFQmhNQ1ZWTXhDekFKQmdOVkJBZ1RBa05CTVJRd0VnWURWUVFIRXd0VFlXNTBZU0JEYkdGeVlURVJNQThHCkExVUVDaE1JUkdGMFlWTjBZWGd4RGpBTUJnTlZCQXNUQlVOc2IzVmtNVlF3VWdZRFZRUURFMHRqYkdsbGJuUXUKTTJOaE5tWXhPV0V0Wm1GbE9DMDBNRGhoTFdKak5EUXRNR0psTkRRNU5EZGpPVFE1TFhWekxYZGxjM1F0TWk1awpZaTVoYzNSeVlTNWtZWFJoYzNSaGVDNWpiMjB3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQkR3QXdnZ0VLCkFvSUJBUUROaFd1RFM3U1dwcTlUWVd4bkM0eUQzMjgvbmlNT29RcE5yZXlLTFN2ZVVBQU1xMm93clB3UlJZYTUKZ3JKUzU1MXkyQjJaYXRTTUNPYXVqL1l6QmVlbk81NHhuRW43ZCtkZVFleVZOWk1RVmo5SzZyNVRIaEhpZnR0Mwo0Nk90R0FIbE1jNFVxNitwV1dzbHgwM0psMjl6TjdDaFZqWmpLVWZ4K2lsUzhUNmlMOTFlRzhsZHovM1l0N3B3CmNTWEhyK0JHejFidXo1d2YrcmtCU0N4NS9oT3NZUXhBMTRZZzA4QktuNERkbURmelZWOFl1ZlR4c2UxWStzemcKbjNxRVB4aGVDQmN2dGZjb0RHbkJUcEJaS0kybmRRRlV2TVJDdDFmalJoTU9QN2dEcGhMQUZpNERyTkpWQTBubwpQc05aa05QWTZNSElPUXBkcTN3MEdablRVai92QWdNQkFBR2pRVEEvTUE0R0ExVWREd0VCL3dRRUF3SUhnREFkCkJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFnWUlLd1lCQlFVSEF3RXdEZ1lEVlIwT0JBY0VCUUVDQXdRR01BMEcKQ1NxR1NJYjNEUUVCQ3dVQUE0SUJBUUNxOWoyRXZYZFFzck9jTVlheUtWL0M0dGw2WTNFOEJCSU1rMHpSZ0tmWgpZVFV2V0RSeFVLY0hDeXFnT1lWWjhQbzlFaVpmL2lFTEEzbEtmd3VwNVZTbVRCODdvUVNTTEx3ZmJ1YnlYcU56ClJzSDdBSlNKZ0drMzJ4cWdpYmZzVU81MzA2SUVtTkRLK3ZkUTZjSk51bUtaeGhvRkZqTHY3QVI2RXBsVDRpKzcKSEprQk5XQnJFejNyaVhNL0VFSnN5V0p4dWJBL3pkcUk4WkI5ZFNJcmZ3NWp5N3lGNWw4ZjNNWjBjNnJzVDluZQpRTXVIeFRBNC95UnIrenlGZ3oyNDlwTHoybHlJT01OTmlxVkNubzVER1ZNSHQ0T08zbnVyT2lIelJUSnVsbDFKCkxvaU1xK2FLVFFITUU4T1ZKRUhvbHgrT242Q3JvSHRLa1Y4SER3WCtsb2syCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
					CaFile:  "/a/b/c",
				},
			},
			err: errors.New("Cannot specify both caData and caFile properties"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewCassandraCluster(tc.cfg)
			if !errors.Is(err, tc.err) {
				assert.Equal(t, tc.err, err)
			}
		})
	}
}
