package kasada

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// CDGenerator generates Kasada x-kpsdk-cd proof-of-work payloads.
type CDGenerator struct {
	ID       string
	Answers  []int
	ST       int64
	WorkTime int64
	RST      int64
	Delta    int
}

// NewCDGenerator creates a CD generator for the given Kasada server timestamp.
func NewCDGenerator(kpsdkST int64) *CDGenerator {
	cd := &CDGenerator{
		ST:       kpsdkST,
		WorkTime: time.Now().UnixNano() / int64(time.Millisecond),
	}
	cd.ID = cd.generateUUID()
	delta := randomInt(50, 150)
	cd.RST = cd.ST + int64(delta)
	cd.Delta = (delta - 1) * -1
	return cd
}

func (cd *CDGenerator) generateUUID() string {
	uuid := make([]byte, 16)
	_, _ = rand.Read(uuid)
	return fmt.Sprintf("%x", uuid)
}

func (cd *CDGenerator) generateJSON() string {
	result := map[string]any{
		"workTime": cd.WorkTime,
		"id":       cd.ID,
		"answers":  cd.Answers,
		"d":        cd.Delta,
		"rst":      cd.RST,
		"st":       cd.ST,
		"duration": fmt.Sprintf("%.1f", math.Floor(1)*9+1),
	}
	jsonData, _ := json.Marshal(result)
	return string(jsonData)
}

func (cd *CDGenerator) byteToHex(bArr []byte) string {
	cArr := "0123456789abcdef"
	var cArr2 string
	for _, b2 := range bArr {
		cArr2 += string(cArr[(b2&240)>>4]) + string(cArr[b2&15])
	}
	return cArr2
}

func (cd *CDGenerator) checkChallenge(strVal string) float64 {
	var b float64
	for _, digit := range strVal[:13] {
		b = b*16 + float64(digit) - 48
	}
	j := b + 1
	return 4.503599627370496e15 / j
}

func (cd *CDGenerator) sha256Digest(message string) []byte {
	hasher := sha256.New()
	hasher.Write([]byte(message))
	return hasher.Sum(nil)
}

func (cd *CDGenerator) baseCDGen() {
	cd.generateCD("tp-v2-input", 10, 2)
}

func (cd *CDGenerator) generateCD(message string, i, i2 int) {
	d := float64(i) / float64(i2)
	digest2 := cd.sha256Digest(fmt.Sprintf("%s, %d, %s", message, cd.WorkTime, cd.ID))
	i3 := 0
	for i3 < i2 {
		i4 := 1
		for {
			digest := cd.sha256Digest(fmt.Sprintf("%d, %s", i4, cd.byteToHex(digest2)))
			e := cd.byteToHex(digest)
			if cd.checkChallenge(e) >= d {
				break
			}
			i4++
		}
		cd.Answers = append(cd.Answers, i4)
		i3++
		digest2 = cd.sha256Digest(fmt.Sprintf("%d, %s", i4, cd.byteToHex(digest2)))
	}
}

// GenerateCD runs the Kasada PoW and returns the x-kpsdk-cd JSON string.
func GenerateCD(kpsdkST int64) string {
	cd := NewCDGenerator(kpsdkST)
	cd.baseCDGen()
	return cd.generateJSON()
}

func randomInt(min, max int) int {
	if max-min == 0 {
		return 0
	}
	return min + rand.Intn(max-min)
}
