package logger

import (
    "context"
    "fmt"
    "os"
    "sync"
    "time"
		"log/slog"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Logger interface {
    Info(msg string, keyvals ...interface{})
    Error(msg string, keyvals ...interface{})
    Debug(msg string, keyvals ...interface{})
    Warn(msg string, keyvals ...interface{})
}

var (
    instance Logger
    once     sync.Once
)

// GetLogger returns the singleton logger instance.
func GetLogger() Logger {
    once.Do(func() {
        var err error
        instance, err = InitLogger()
        if err != nil {
            // ロガーの初期化に失敗した場合、標準のロガーを使用
            fmt.Fprintf(os.Stderr, "Failed to initialize configured logger: %v\nUsing default slog logger.\n", err)
            instance = NewSlogLogger()
            instance.Error("Failed to initialize configured logger, using default slog logger", "error", err)
        }
    })
    return instance
}

// InitLogger initializes and returns the appropriate Logger based on environment variables.
// Supported LOGGER_TYPE values: "slog", "dynamodb"
func InitLogger() (Logger, error) {
    loggerType := os.Getenv("LOGGER_TYPE")
    if loggerType == "" {
        loggerType = "slog"
    }

    switch loggerType {
    case "slog":
        return NewSlogLogger(), nil
    case "dynamodb":
        tableName := os.Getenv("DYNAMODB_TABLE")
        if tableName == "" {
            return nil, fmt.Errorf("DYNAMODB_TABLE environment variable is required for dynamodb logger")
        }
        return NewDynamoDBLogger(tableName)
    default:
        return nil, fmt.Errorf("unsupported LOGGER_TYPE: %s", loggerType)
    }
}

// SlogLogger wraps slog.Logger to implement the Logger interface.
type SlogLogger struct {
    logger *slog.Logger
}

func NewSlogLogger() *SlogLogger {
    // JSTタイムゾーンを設定
    jst := time.FixedZone("Asia/Tokyo", 9*60*60)

    handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        AddSource: true,
        Level:     slog.LevelInfo,
        ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
            if a.Key == "time" && a.Value.Kind() == slog.KindTime {
                return slog.String(a.Key, a.Value.Time().In(jst).Format("15:04:05.000"))
            }
            return a
        },
    })

    logger := slog.New(handler)
    return &SlogLogger{logger: logger}
}

func (l *SlogLogger) Info(msg string, keyvals ...interface{}) {
    l.logger.Info(msg, keyvals...)
}

func (l *SlogLogger) Error(msg string, keyvals ...interface{}) {
    l.logger.Error(msg, keyvals...)
}

func (l *SlogLogger) Debug(msg string, keyvals ...interface{}) {
    l.logger.Debug(msg, keyvals...)
}

func (l *SlogLogger) Warn(msg string, keyvals ...interface{}) {
    l.logger.Warn(msg, keyvals...)
}

// DynamoDBLogger implements the Logger interface and writes logs to DynamoDB.
type DynamoDBLogger struct {
    client     *dynamodb.Client
    tableName  string
}

func NewDynamoDBLogger(tableName string) (*DynamoDBLogger, error) {
    cfg, err := config.LoadDefaultConfig(context.TODO())
    if err != nil {
        return nil, fmt.Errorf("unable to load AWS SDK config, %v", err)
    }

    client := dynamodb.NewFromConfig(cfg)
    return &DynamoDBLogger{
        client:    client,
        tableName: tableName,
    }, nil
}

func (l *DynamoDBLogger) Info(msg string, keyvals ...interface{}) {
    l.log("INFO", msg, keyvals...)
}

func (l *DynamoDBLogger) Error(msg string, keyvals ...interface{}) {
    l.log("ERROR", msg, keyvals...)
}

func (l *DynamoDBLogger) Debug(msg string, keyvals ...interface{}) {
    l.log("DEBUG", msg, keyvals...)
}

func (l *DynamoDBLogger) Warn(msg string, keyvals ...interface{}) {
    l.log("WARN", msg, keyvals...)
}

func (l *DynamoDBLogger) log(level, msg string, keyvals ...interface{}) {
    logItem := map[string]types.AttributeValue{
        "Timestamp":      &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339Nano)},
        "Level":          &types.AttributeValueMemberS{Value: level},
        "Message":        &types.AttributeValueMemberS{Value: msg},
    }

    // 追加のキー値ペアを処理
    for i := 0; i < len(keyvals)-1; i += 2 {
        key := fmt.Sprintf("%v", keyvals[i])
        value := fmt.Sprintf("%v", keyvals[i+1])
        logItem[key] = &types.AttributeValueMemberS{Value: value}
    }

    // DynamoDB にログ項目を挿入
    _, err := l.client.PutItem(context.TODO(), &dynamodb.PutItemInput{
        TableName: aws.String(l.tableName),
        Item:      logItem,
    })

    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to write log to DynamoDB: %v\n", err)
    }
}
