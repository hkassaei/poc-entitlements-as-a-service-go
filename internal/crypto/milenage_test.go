package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// 3GPP TS 35.207 Test Set 1
func TestMilenageTestSet1(t *testing.T) {
	ki := mustHex("465b5ce8b199b49faa5f0a2ee238a6bc")
	op := mustHex("cdc202d5123e20f62b6d676ac72cb318")
	rand := mustHex("23553cbe9637a89d218ae64dae47bf35")
	sqn := mustHex("ff9bb4d0b607")
	amf := mustHex("b9b9")

	t.Run("computes correct OPc", func(t *testing.T) {
		opc, err := ComputeOPc(ki, op)
		require.NoError(t, err)
		assert.Equal(t, "cd63cb71954a9f4e48a5994e37a02baf", hex.EncodeToString(opc))
	})

	t.Run("generates correct f1 (MAC-A)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		macA := hex.EncodeToString(vectors.AUTN[8:16])
		assert.Equal(t, "4a9ffac354dfafb3", macA)
	})

	t.Run("generates correct f2 (RES/XRES)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "a54211d5e3ba50bf", hex.EncodeToString(vectors.XRES))
	})

	t.Run("generates correct f3 (CK)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "b40ba9a3c58b2a05bbf0d987b21bf8cb", hex.EncodeToString(vectors.CK))
	})

	t.Run("generates correct f4 (IK)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "f769bcd751044604127672711c6d3441", hex.EncodeToString(vectors.IK))
	})

	t.Run("generates correct f5 (AK)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "aa689c648370", hex.EncodeToString(vectors.AK))
	})
}

// 3GPP TS 35.207 Test Set 2
func TestMilenageTestSet2(t *testing.T) {
	ki := mustHex("0396eb317b6d1c36f19c1c84cd6ffd16")
	op := mustHex("ff53bade17df5d4e793073ce9d7579fa")
	rand := mustHex("c00d603103dcee52c4478119494202e8")
	sqn := mustHex("fd8eef40df7d")
	amf := mustHex("af17")

	t.Run("computes correct OPc", func(t *testing.T) {
		opc, err := ComputeOPc(ki, op)
		require.NoError(t, err)
		assert.Equal(t, "53c15671c60a4b731c55b4a441c0bde2", hex.EncodeToString(opc))
	})

	t.Run("generates correct f1 (MAC-A)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		macA := hex.EncodeToString(vectors.AUTN[8:16])
		assert.Equal(t, "5df5b31807e258b0", macA)
	})

	t.Run("generates correct f2 (RES/XRES)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "d3a628ed988620f0", hex.EncodeToString(vectors.XRES))
	})

	t.Run("generates correct f3 (CK)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "58c433ff7a7082acd424220f2b67c556", hex.EncodeToString(vectors.CK))
	})

	t.Run("generates correct f4 (IK)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "21a8c1f929702adb3e738488b9f5c5da", hex.EncodeToString(vectors.IK))
	})

	t.Run("generates correct f5 (AK)", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		assert.Equal(t, "c47783995f72", hex.EncodeToString(vectors.AK))
	})
}

func TestMilenageResyncFunctions(t *testing.T) {
	ki := mustHex("465b5ce8b199b49faa5f0a2ee238a6bc")
	op := mustHex("cdc202d5123e20f62b6d676ac72cb318")
	rand := mustHex("23553cbe9637a89d218ae64dae47bf35")

	t.Run("f5* produces a 6-byte AK*", func(t *testing.T) {
		akStar, err := F5Star(ki, rand, op)
		require.NoError(t, err)
		assert.Len(t, akStar, 6)
	})

	t.Run("f5* produces AK* different from f5 AK", func(t *testing.T) {
		vectors, err := GenerateVectorsWithRAND(ki, op, rand, mustHex("ff9bb4d0b607"), mustHex("b9b9"))
		require.NoError(t, err)
		akStar, err := F5Star(ki, rand, op)
		require.NoError(t, err)
		assert.NotEqual(t, hex.EncodeToString(vectors.AK), hex.EncodeToString(akStar))
	})

	t.Run("f5* is deterministic", func(t *testing.T) {
		akStar1, err := F5Star(ki, rand, op)
		require.NoError(t, err)
		akStar2, err := F5Star(ki, rand, op)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(akStar1), hex.EncodeToString(akStar2))
	})

	t.Run("f1* produces an 8-byte MAC-S", func(t *testing.T) {
		sqnMs := mustHex("000000000100")
		amfZero := make([]byte, 2)
		macS, err := F1Star(ki, rand, sqnMs, amfZero, op)
		require.NoError(t, err)
		assert.Len(t, macS, 8)
	})

	t.Run("f1* produces MAC-S different from f1 MAC-A", func(t *testing.T) {
		sqn := mustHex("ff9bb4d0b607")
		amf := mustHex("b9b9")
		amfZero := make([]byte, 2)

		vectors, err := GenerateVectorsWithRAND(ki, op, rand, sqn, amf)
		require.NoError(t, err)
		macA := hex.EncodeToString(vectors.AUTN[8:16])
		macS, err := F1Star(ki, rand, sqn, amfZero, op)
		require.NoError(t, err)

		assert.NotEqual(t, macA, hex.EncodeToString(macS))
	})

	t.Run("f1* is deterministic", func(t *testing.T) {
		sqnMs := mustHex("000000000200")
		amfZero := make([]byte, 2)
		macS1, err := F1Star(ki, rand, sqnMs, amfZero, op)
		require.NoError(t, err)
		macS2, err := F1Star(ki, rand, sqnMs, amfZero, op)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(macS1), hex.EncodeToString(macS2))
	})
}

func TestValidateAUTS(t *testing.T) {
	ki := mustHex("465b5ce8b199b49faa5f0a2ee238a6bc")
	op := mustHex("cdc202d5123e20f62b6d676ac72cb318")
	rand := mustHex("23553cbe9637a89d218ae64dae47bf35")

	t.Run("returns valid=true for correct AUTS", func(t *testing.T) {
		sqnMs := mustHex("000000000100")
		auts, err := GenerateAUTS(ki, rand, sqnMs, op)
		require.NoError(t, err)
		result, err := ValidateAUTS(ki, rand, auts, op)
		require.NoError(t, err)
		assert.True(t, result.Valid)
		require.NotNil(t, result.SqnMs)
		assert.Equal(t, hex.EncodeToString(sqnMs), hex.EncodeToString(result.SqnMs))
	})

	t.Run("returns valid=false for tampered AUTS (modified MAC-S)", func(t *testing.T) {
		sqnMs := mustHex("000000000100")
		auts, err := GenerateAUTS(ki, rand, sqnMs, op)
		require.NoError(t, err)
		auts[10] ^= 0xff
		result, err := ValidateAUTS(ki, rand, auts, op)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Nil(t, result.SqnMs)
	})

	t.Run("returns valid=false for tampered AUTS (modified concealed-SQN)", func(t *testing.T) {
		sqnMs := mustHex("000000000100")
		auts, err := GenerateAUTS(ki, rand, sqnMs, op)
		require.NoError(t, err)
		auts[3] ^= 0xff
		result, err := ValidateAUTS(ki, rand, auts, op)
		require.NoError(t, err)
		assert.False(t, result.Valid)
	})

	t.Run("returns valid=false for wrong RAND", func(t *testing.T) {
		sqnMs := mustHex("000000000100")
		auts, err := GenerateAUTS(ki, rand, sqnMs, op)
		require.NoError(t, err)
		wrongRand := mustHex("0000000000000000000000000000dead")
		result, err := ValidateAUTS(ki, wrongRand, auts, op)
		require.NoError(t, err)
		assert.False(t, result.Valid)
	})

	t.Run("returns valid=false for wrong Ki", func(t *testing.T) {
		sqnMs := mustHex("000000000100")
		auts, err := GenerateAUTS(ki, rand, sqnMs, op)
		require.NoError(t, err)
		wrongKi := mustHex("deadbeefdeadbeefdeadbeefdeadbeef")
		result, err := ValidateAUTS(wrongKi, rand, auts, op)
		require.NoError(t, err)
		assert.False(t, result.Valid)
	})

	t.Run("returns valid=false for invalid AUTS length", func(t *testing.T) {
		result, err := ValidateAUTS(ki, rand, []byte("tooshort"), op)
		require.NoError(t, err)
		assert.False(t, result.Valid)
	})

	t.Run("round-trip: generate AUTS then validate extracts same SQN_MS", func(t *testing.T) {
		testSqns := []string{
			"000000000000",
			"000000000001",
			"0000000000ff",
			"ffffffffffff",
			"123456789abc",
		}
		for _, sqnHex := range testSqns {
			sqnMs := mustHex(sqnHex)
			auts, err := GenerateAUTS(ki, rand, sqnMs, op)
			require.NoError(t, err)
			result, err := ValidateAUTS(ki, rand, auts, op)
			require.NoError(t, err)
			assert.True(t, result.Valid, "SQN: %s", sqnHex)
			assert.Equal(t, sqnHex, hex.EncodeToString(result.SqnMs))
		}
	})
}

func TestGenerateAUTS(t *testing.T) {
	ki := mustHex("465b5ce8b199b49faa5f0a2ee238a6bc")
	op := mustHex("cdc202d5123e20f62b6d676ac72cb318")
	rand := mustHex("23553cbe9637a89d218ae64dae47bf35")

	t.Run("produces a 14-byte AUTS", func(t *testing.T) {
		sqnMs := mustHex("000000000100")
		auts, err := GenerateAUTS(ki, rand, sqnMs, op)
		require.NoError(t, err)
		assert.Len(t, auts, 14)
	})

	t.Run("AUTS structure: first 6 bytes concealed-SQN, next 8 bytes MAC-S", func(t *testing.T) {
		sqnMs := mustHex("abcdef012345")
		auts, err := GenerateAUTS(ki, rand, sqnMs, op)
		require.NoError(t, err)

		concealedSqn := auts[:6]
		macS := auts[6:14]

		// Verify concealed-SQN = SQN_MS XOR AK*
		akStar, err := F5Star(ki, rand, op)
		require.NoError(t, err)
		expectedConcealedSqn := XOR(sqnMs, akStar)
		assert.Equal(t, hex.EncodeToString(expectedConcealedSqn), hex.EncodeToString(concealedSqn))

		// Verify MAC-S
		amfZero := make([]byte, 2)
		expectedMacS, err := F1Star(ki, rand, sqnMs, amfZero, op)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(expectedMacS), hex.EncodeToString(macS))
	})
}
