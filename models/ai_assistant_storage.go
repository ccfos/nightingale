package models

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/gorm"
)

// ==================== DB row structs (used by both GORM AutoMigrate and queries) ====================

type AssistantChatRow struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ChatID    string `gorm:"column:chat_id;type:varchar(255);not null;uniqueIndex:uk_chat_id"`
	UserID    int64  `gorm:"column:user_id;not null;default:0;index:idx_ac_user_id"`
	UpdatedAt int64  `gorm:"column:updated_at;not null;default:0;index:idx_ac_updated_at"`
	Data      string `gorm:"column:data;type:text"`
}

func (AssistantChatRow) TableName() string { return "ai_assistant_chat" }

type AssistantMessageRow struct {
	Id     int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ChatID string `gorm:"column:chat_id;type:varchar(255);not null;uniqueIndex:uk_chat_seq,priority:1"`
	SeqID  int64  `gorm:"column:seq_id;not null;default:0;uniqueIndex:uk_chat_seq,priority:2"`
	Data   string `gorm:"column:data;type:text"`
	Extra  string `gorm:"column:extra;type:text"`
	Status int    `gorm:"column:status;type:int;not null;default:0;index:idx_am_status"`
}

func (AssistantMessageRow) TableName() string { return "ai_assistant_message" }

// MysqlAssistantMessageRow is used only for MySQL AutoMigrate to upgrade
// data/extra columns from TEXT (64KB) to MEDIUMTEXT (16MB), because reasoning
// content can be very long. PostgreSQL/SQLite text is already unlimited;
// SQLite treats mediumtext as text affinity so the else branch covers both.
type MysqlAssistantMessageRow struct {
	Id     int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ChatID string `gorm:"column:chat_id;type:varchar(255);not null;uniqueIndex:uk_chat_seq,priority:1"`
	SeqID  int64  `gorm:"column:seq_id;not null;default:0;uniqueIndex:uk_chat_seq,priority:2"`
	Data   string `gorm:"column:data;type:mediumtext"`
	Extra  string `gorm:"column:extra;type:mediumtext"`
	Status int    `gorm:"column:status;type:int;not null;default:0;index:idx_am_status"`
}

func (MysqlAssistantMessageRow) TableName() string { return "ai_assistant_message" }

// ==================== Gzip helpers ====================

func gzipBase64Encode(data []byte) (string, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return "", fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("gzip close: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func gzipBase64Decode(input string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	var out bytes.Buffer
	if _, err := io.Copy(&out, gz); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func encodeChat(chat AssistantChat) (AssistantChatRow, error) {
	data, err := gzipBase64Encode(mustJSON(chat))
	if err != nil {
		return AssistantChatRow{}, fmt.Errorf("encode chat: %w", err)
	}
	return AssistantChatRow{
		ChatID:    chat.ChatID,
		UserID:    chat.UserID,
		UpdatedAt: chat.LastUpdate,
		Data:      data,
	}, nil
}

func decodeChat(row *AssistantChatRow) (*AssistantChat, error) {
	if row == nil || row.Data == "" {
		return nil, nil
	}
	decoded, err := gzipBase64Decode(row.Data)
	if err != nil {
		return nil, fmt.Errorf("gzip decode chat: %w", err)
	}
	var chat AssistantChat
	if err := json.Unmarshal(decoded, &chat); err != nil {
		return nil, err
	}
	return &chat, nil
}

func encodeMessage(msg AssistantMessage) (AssistantMessageRow, error) {
	data, err := gzipBase64Encode(mustJSON(msg))
	if err != nil {
		return AssistantMessageRow{}, fmt.Errorf("encode message data: %w", err)
	}
	extra, err := gzipBase64Encode(mustJSON(msg.Extra))
	if err != nil {
		return AssistantMessageRow{}, fmt.Errorf("encode message extra: %w", err)
	}
	return AssistantMessageRow{
		ChatID: msg.ChatID,
		SeqID:  msg.SeqID,
		Data:   data,
		Extra:  extra,
	}, nil
}

func decodeMessage(row *AssistantMessageRow) (*AssistantMessage, error) {
	if row == nil || row.Data == "" {
		return nil, nil
	}
	decoded, err := gzipBase64Decode(row.Data)
	if err != nil {
		return nil, err
	}
	var msg AssistantMessage
	if err := json.Unmarshal(decoded, &msg); err != nil {
		return nil, err
	}
	if row.Extra != "" {
		decodedExtra, err := gzipBase64Decode(row.Extra)
		if err == nil {
			json.Unmarshal(decodedExtra, &msg.Extra)
		}
	}
	return &msg, nil
}

// ==================== Chat Storage ====================

func AssistantChatSet(c *ctx.Context, chat AssistantChat) error {
	row, err := encodeChat(chat)
	if err != nil {
		return err
	}
	// Try update first; if no rows affected, insert.
	result := DB(c).Model(&AssistantChatRow{}).
		Where("chat_id = ?", chat.ChatID).
		Updates(map[string]any{"data": row.Data, "updated_at": row.UpdatedAt, "user_id": row.UserID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return DB(c).Create(&row).Error
	}
	return nil
}

func AssistantChatGet(c *ctx.Context, chatID string) (*AssistantChat, error) {
	var row AssistantChatRow
	err := DB(c).Where("chat_id = ?", chatID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return decodeChat(&row)
}

func AssistantChatGetsByUserID(c *ctx.Context, userID int64) ([]AssistantChat, error) {
	var rows []AssistantChatRow
	err := DB(c).Where("user_id = ?", userID).Order("updated_at desc").Find(&rows).Error
	if err != nil {
		return nil, err
	}

	var chats []AssistantChat
	for i := range rows {
		chat, err := decodeChat(&rows[i])
		if err != nil || chat == nil {
			continue
		}
		if !chat.IsNew {
			chats = append(chats, *chat)
		}
	}
	return chats, nil
}

func AssistantChatCheckOwner(c *ctx.Context, chatID string, userID int64) (*AssistantChat, error) {
	chat, err := AssistantChatGet(c, chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, fmt.Errorf("chat not found")
	}
	if chat.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}
	return chat, nil
}

func AssistantChatDelete(c *ctx.Context, chatID string) error {
	if err := DB(c).Where("chat_id = ?", chatID).Delete(&AssistantMessageRow{}).Error; err != nil {
		return err
	}
	return DB(c).Where("chat_id = ?", chatID).Delete(&AssistantChatRow{}).Error
}

// ==================== Message Storage ====================

func AssistantMessageMaxSeqID(c *ctx.Context, chatID string) (int64, error) {
	var row AssistantMessageRow
	err := DB(c).Where("chat_id = ?", chatID).Order("seq_id desc").Select("seq_id").First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.SeqID, nil
}

func AssistantMessageSet(c *ctx.Context, msg AssistantMessage) error {
	row, err := encodeMessage(msg)
	if err != nil {
		return err
	}
	// Try update first; if no rows affected, insert.
	result := DB(c).Model(&AssistantMessageRow{}).
		Where("chat_id = ? AND seq_id = ?", msg.ChatID, msg.SeqID).
		Updates(map[string]any{"data": row.Data, "extra": row.Extra})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return DB(c).Create(&row).Error
	}
	return nil
}

func AssistantMessageGet(c *ctx.Context, chatID string, seqID int64) (*AssistantMessage, error) {
	var row AssistantMessageRow
	err := DB(c).Where("chat_id = ? AND seq_id = ?", chatID, seqID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return decodeMessage(&row)
}

func AssistantMessageGetsByChat(c *ctx.Context, chatID string) ([]AssistantMessage, error) {
	var rows []AssistantMessageRow
	err := DB(c).Where("chat_id = ?", chatID).Order("seq_id asc").Find(&rows).Error
	if err != nil {
		return nil, err
	}

	var msgs []AssistantMessage
	for i := range rows {
		if AssistantMessageStatus(rows[i].Status) == MessageStatusCancel {
			continue
		}
		msg, err := decodeMessage(&rows[i])
		if err != nil || msg == nil {
			continue
		}
		msgs = append(msgs, *msg)
	}
	return msgs, nil
}

func AssistantMessageSetStatus(c *ctx.Context, chatID string, seqID int64, status AssistantMessageStatus) error {
	return DB(c).Model(&AssistantMessageRow{}).
		Where("chat_id = ? AND seq_id = ?", chatID, seqID).
		Update("status", int(status)).Error
}
