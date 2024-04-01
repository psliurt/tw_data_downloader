package env

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"reflect"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/spf13/viper"

	b64 "encoding/base64"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

func setUpZap() {
	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   "./log/tw_data_downloader.log",
		MaxSize:    30,  // megabytes
		MaxBackups: 300, // backup files
		MaxAge:     31,  // days
	})
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.CallerKey = "caller"
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		w,
		zap.InfoLevel,
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	defer logger.Sync()

	zap.ReplaceGlobals(logger)
	zap.L().Info("zap started")

}

func loadServicePort() int {
	return viper.GetInt("serviceport")
}

type DecimalCodec struct{}

func (dc *DecimalCodec) EncodeValue(ectx bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	// Use reflection to convert the reflect.Value to decimal.Decimal.
	dec, ok := val.Interface().(decimal.Decimal)
	if !ok {
		return fmt.Errorf("value %v to encode is not of type decimal.Decimal", val)
	}

	// Convert decimal.Decimal to primitive.Decimal128.
	primDec, err := primitive.ParseDecimal128(dec.String())
	if err != nil {
		return fmt.Errorf("error converting decimal.Decimal %v to primitive.Decimal128: %v", dec, err)
	}
	return vw.WriteDecimal128(primDec)
}

func (dc *DecimalCodec) DecodeValue(ectx bsoncodec.DecodeContext, vr bsonrw.ValueReader, val reflect.Value) error {
	// Read primitive.Decimal128 from the ValueReader.
	primDec, err := vr.ReadDecimal128()
	if err != nil {
		return fmt.Errorf("error reading primitive.Decimal128 from ValueReader: %v", err)
	}

	// Convert primitive.Decimal128 to decimal.Decimal.
	dec, err := decimal.NewFromString(primDec.String())
	if err != nil {
		return fmt.Errorf("error converting primitive.Decimal128 %v to decimal.Decimal: %v", primDec, err)
	}

	// Set val to the decimal.Decimal value contained in dec.
	val.Set(reflect.ValueOf(dec))
	return nil
}

func encryptData(plainText string, key []byte, iv []byte) string {
	blk, err := aes.NewCipher(key)
	if err != nil {
		zap.L().Error("encrypt data error", zap.Error(err))
		return ""
	}

	blkMode := cipher.NewCBCEncrypter(blk, iv)
	plainTextBytes := []byte(plainText)

	inputPadding, err := pkcs7Pad(plainTextBytes, blk.BlockSize())

	encryptOutput := make([]byte, len(inputPadding))
	if err != nil {
		zap.L().Error("pad error", zap.Error(err))
		return ""
	}
	blkMode.CryptBlocks(encryptOutput, inputPadding)

	encryptedData := b64.RawURLEncoding.EncodeToString(encryptOutput)
	return encryptedData
}

func decryptData(encryptedb64String string, key []byte, iv []byte) string {
	encryptedBytes, err := b64.RawURLEncoding.DecodeString(encryptedb64String)
	if err != nil {
		zap.L().Error("decode encrypted base64 string error", zap.String("encstr", encryptedb64String), zap.Error(err))
		return ""
	}
	deBlk, err := aes.NewCipher(key)
	if err != nil {
		zap.L().Error("new aes256 cipher error", zap.Error(err))
		return ""
	}

	deMode := cipher.NewCBCDecrypter(deBlk, iv)

	decryptedBytes := make([]byte, len(encryptedBytes))

	deMode.CryptBlocks(decryptedBytes, encryptedBytes)
	//加密後的資料長度 - 最後一個Byte的數字就是 加密前的文字的長度
	decryptedBytesRealLength := len(encryptedBytes) - int(decryptedBytes[len(decryptedBytes)-1])

	plainText := string(decryptedBytes[:decryptedBytesRealLength])

	return plainText
}

func pkcs7Pad(b []byte, blocksize int) ([]byte, error) {
	if blocksize <= 0 {
		zap.L().Error("the block size is zero")
		return nil, errors.New("block size is zero")
	}
	if b == nil || len(b) == 0 {
		zap.L().Error("plain text is nil or null, or empty string")
		return nil, errors.New("input data (plain text) is nil or length is zero")
	}
	n := blocksize - (len(b) % blocksize)
	pb := make([]byte, len(b)+n)
	copy(pb, b)
	copy(pb[len(b):], bytes.Repeat([]byte{byte(n)}, n))
	return pb, nil
}
