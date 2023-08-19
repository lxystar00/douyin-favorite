package dao

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

const dbName string = "mysql"
const dbConnect string = "root:123456@tcp(127.0.0.1:3306)/douyin" //设置数据库连接参数

func FavoriteTableChange(tableName string, userID int64, videoID int64, action bool) {
	db, err := sql.Open(dbName, dbConnect)
	if err != nil {
		fmt.Println("数据库连接失败")
		return
	}
	defer db.Close()

	if action {
		// 根据用户id和视频id在点赞关系表中查询对应的结果
		checkExistSql := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_id=%d AND video_id=%d", tableName, userID, videoID)
		var count int
		err := db.QueryRow(checkExistSql).Scan(&count)
		if err != nil {
			fmt.Println("查询记录失败:", err)
			return
		}

		if count > 0 {
			// 存在则删除
			deleteSql := fmt.Sprintf("DELETE FROM %s WHERE user_id=%d AND video_id=%d", tableName, userID, videoID)
			_, err := db.Exec(deleteSql)
			if err != nil {
				fmt.Println("删除记录失败:", err)
				return
			}
		}

		// 增加
		insertSql := fmt.Sprintf("INSERT INTO %s (user_id, video_id, created_at) VALUES (%d, %d, NOW())", tableName, userID, videoID)
		_, err = db.Exec(insertSql)
		if err != nil {
			fmt.Println("插入记录失败:", err)
			return
		}
	} else {
		// 删除
		deleteSql := fmt.Sprintf("DELETE FROM %s WHERE user_id=%d AND video_id=%d", tableName, userID, videoID)
		_, err := db.Exec(deleteSql)
		if err != nil {
			fmt.Println("删除记录失败:", err)
			return
		}
	}
}

// 修改用户的 favorite_count
func ChangeUserFavoriteCount(tableName string, userID int64, action bool) {
	db, err := sql.Open(dbName, dbConnect)
	if err != nil {
		fmt.Println("数据库连接失败")
		return
	}

	var changeValue int
	if action {
		changeValue = 1
	} else {
		changeValue = -1
	}

	Sql := fmt.Sprintf("update %s set favorite_count = favorite_count + %d where id = %d", tableName, changeValue, userID)
	_, err = db.Exec(Sql)
	if err != nil {
		fmt.Println("更新 favorite_count 失败:", err)
	}

	defer db.Close()
}

// 修改视频的 likes_count
func ChangeVideoLikesCount(tableName string, videoID int64, action bool) {
	db, err := sql.Open(dbName, dbConnect)
	if err != nil {
		fmt.Println("数据库连接失败")
		return
	}

	var changeValue int
	if action {
		changeValue = 1
	} else {
		changeValue = -1
	}

	Sql := fmt.Sprintf("update %s set likes_count = likes_count + %d where video_id = %d", tableName, changeValue, videoID)
	_, err = db.Exec(Sql)
	if err != nil {
		fmt.Println("更新 likes_count 失败:", err)
	}

	defer db.Close()
}
