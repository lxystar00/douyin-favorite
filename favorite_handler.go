package v1

import (
	//"database/sql"
	"douyin/database"
	"douyin/database/models"
	"fmt"
	"time"

	//"log"
	"net/http"
	"strconv"

	"douyin/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
)

/*const (
	successCode = 0
	errorCode   = 1
)*/

//const dbName string = "mysql"
//const dbConnect string = "root:123456@tcp(127.0.0.1:3306)/douyin?charset=utf8&parseTime=true" //设置数据库连接参数

func Response(ctx *gin.Context, httpStatus int, v interface{}) {
	ctx.JSON(httpStatus, v)
}

type FavActionParams struct {
	Token      string `form:"token" binding:"required"`
	VideoId    int64  `form:"video_id" binding:"required"`
	ActionType int8   `form:"action_type" binding:"required,oneof=1 2"`
}

type FavListResponse struct {
	StatusCode int            `json:"status_code"`
	StatusMsg  string         `json:"status_msg"`
	VideoList  []models.Video `json:"video_list"`
}

// 点赞视频
func FavoriteAction(ctx *gin.Context) {
	var favInfo FavActionParams
	err := ctx.ShouldBind(&favInfo)
	if err != nil {
		Response(ctx, 400, gin.H{"error": err.Error()})
		return
	}
	tokenUids, _ := ctx.Get("user_id")
	tokenUid, _ := tokenUids.(int64)

	if err != nil {
		Response(ctx, 500, gin.H{"error": err.Error()})
		return
	}

	redisPool := middleware.RedisPool
	if redisPool != nil {
		fmt.Println("get")
	}
	err = FavoriteActionDo(tokenUid, favInfo.VideoId, favInfo.ActionType, redisPool)

	if err != nil {
		Response(ctx, 500, gin.H{"error": err.Error()})
		return
	}
	Response(ctx, 200, gin.H{"message": "success"})
	// 获取 Redis 连接池

}

func FavoriteActionDo(uid, vid int64, action int8, redisPool *redis.Pool) error {
	var authoruid int64
	err := database.DB.Table("video").Where("video_id = ?", vid).Pluck("author_user_id", &authoruid).Error
	if err != nil {
		fmt.Println("查询视频作者id失败")
	}

	if action == 1 {
		FavoriteTableChange("favorite", uid, vid, true)
		ChangeUserFavoriteCount("user", uid, "favorite_count", true)
		ChangeUserFavoriteCount("user", authoruid, "total_favorited", true)
		ChangeVideoLikesCount("video", vid, true)
		CacheFavoriteAction(uid, vid, true, redisPool)
	} else {
		FavoriteTableChange("favorite", uid, vid, false)
		ChangeUserFavoriteCount("user", uid, "favorite_count", false)
		ChangeUserFavoriteCount("user", authoruid, "total_favorited", false)
		ChangeVideoLikesCount("video", vid, false)
		CacheFavoriteAction(uid, vid, false, redisPool)

	}

	return nil
}

func CacheFavoriteAction(uid, vid int64, action bool, redisPool *redis.Pool) error {
	conn := redisPool.Get() //重用已有的连接
	defer conn.Close()

	var authoruid int64
	// 获取视频对应的用户ID
	authoruid, err := redis.Int64(conn.Do("HGET", "video:"+strconv.FormatInt(vid, 10), "author_user_id"))

	if err != nil {
		return err
	}

	//使用 Redis 的字符串（String）来存储用户表和视频表的信息，使用集合（Set）来存储点赞关系。

	if action {
		conn.Send("SADD", "user:"+strconv.FormatInt(uid, 10)+":likes", vid)            //存储点赞关系
		conn.Send("INCR", "user:"+strconv.FormatInt(uid, 10)+":favorite_count")        //增加用户点赞数
		conn.Send("INCR", "video:"+strconv.FormatInt(vid, 10)+":likes_count")          //增加用户点赞量
		conn.Send("INCR", "user:"+strconv.FormatInt(authoruid, 10)+":total_favorited") //增加视频作者的被点赞量

	} else {
		conn.Send("SREM", "user:"+strconv.FormatInt(uid, 10)+":likes", vid)
		conn.Send("DECR", "user:"+strconv.FormatInt(uid, 10)+":favorite_count")
		conn.Send("DECR", "video:"+strconv.FormatInt(vid, 10)+":likes_count")
		conn.Send("DECR", "user:"+strconv.FormatInt(authoruid, 10)+":total_favorited")
	}
	conn.Flush()
	return nil
}

func FavoriteTableChange(tableName string, userID int64, videoID int64, action bool) {
	/*db, err := sql.Open(dbName, dbConnect)
	if err != nil {
		fmt.Println("数据库连接失败")
		return
	}
	defer db.Close()
	*/
	if action {
		// 根据用户id和视频id在点赞关系表中查询对应的结果
		//checkExistSql := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_id=%d AND video_id=%d", tableName, userID, videoID)
		var count int
		//err := db.QueryRow(checkExistSql).Scan(&count)
		err := database.DB.Table(tableName).Where("user_id=? AND video_id=?", userID, videoID).Count(&count).Error
		if err != nil {
			fmt.Println("查询记录失败:", err)
			return
		}

		if count > 0 {
			// 存在则删除
			//deleteSql := fmt.Sprintf("DELETE FROM %s WHERE user_id=%d AND video_id=%d", tableName, userID, videoID)
			//_, err := db.Exec(deleteSql)
			err := database.DB.Table(tableName).Where("user_id=? AND video_id=?", userID, videoID).Delete(&models.Favorite{}).Error
			if err != nil {
				fmt.Println("删除记录失败:", err)
				return
			}
		}

		fav := models.Favorite{
			UserID:    userID,
			VideoID:   videoID,
			CreatedAt: time.Now(),
		}
		// 增加
		//insertSql := fmt.Sprintf("INSERT INTO %s (user_id, video_id, created_at) VALUES (%d, %d, NOW())", tableName, userID, videoID)
		//_, err = db.Exec(insertSql)
		err = database.DB.Table(tableName).Create(&fav).Error
		if err != nil {
			fmt.Println("插入记录失败:", err)
			return
		}
	} else {
		// 删除
		//deleteSql := fmt.Sprintf("DELETE FROM %s WHERE user_id=%d AND video_id=%d", tableName, userID, videoID)
		//_, err := db.Exec(deleteSql)
		err := database.DB.Table(tableName).Where("user_id=? AND video_id=?", userID, videoID).Delete(&models.Favorite{}).Error
		if err != nil {
			fmt.Println("删除记录失败:", err)
			return
		}
	}
}

// 修改用户的 favorite_count
func ChangeUserFavoriteCount(tableName string, userID int64, userORauther string, action bool) {
	/*db, err := sql.Open(dbName, dbConnect)
	if err != nil {
		fmt.Println("数据库连接失败")
		return
	}*/

	var changeValue int
	if action {
		changeValue = 1
	} else {
		changeValue = -1
	}

	//Sql := fmt.Sprintf("update %s set favorite_count = favorite_count + %d where id = %d", tableName, changeValue, userID)
	//_, err = db.Exec(Sql)
	//err := database.DB.Table(tableName).Where("id=?", userID).Update("favorite_count", gorm.Expr("favorite_count+?", changeValue)).Error
	err := database.DB.Table(tableName).Where("id=?", userID).Update(userORauther, gorm.Expr(userORauther+" + ?", changeValue)).Error
	if err != nil {
		fmt.Println("更新 favorite_count 失败:", err)
	}

	//defer db.Close()
}

// 修改视频的 likes_count
func ChangeVideoLikesCount(tableName string, videoID int64, action bool) {
	/*db, err := sql.Open(dbName, dbConnect)
	if err != nil {
		fmt.Println("数据库连接失败")
		return
	}*/

	var changeValue int
	if action {
		changeValue = 1
	} else {
		changeValue = -1
	}

	//Sql := fmt.Sprintf("update %s set likes_count = likes_count + %d where video_id = %d", tableName, changeValue, videoID)
	//_, err = db.Exec(Sql)
	err := database.DB.Table(tableName).Where("video_id=?", videoID).Update("likes", gorm.Expr("likes+?", changeValue)).Error
	if err != nil {
		fmt.Println("更新 likes_count 失败:", err)
	}

	//defer db.Close()
}

// 获取点赞列表
func FavoriteList(ctx *gin.Context) {
	uID := ctx.Query("user_id")
	//token := ctx.Query("token")
	userID, _ := strconv.ParseInt(uID, 10, 64)

	// 验证token是否有效
	/*if err := validateToken(userID, token); err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "无效的Token"})
		return
	}*/

	videoId := GetVideoId(userID)
	fmt.Println("videoId==", videoId)
	resp, err := GetVideoById(videoId)
	if err != nil {
		fmt.Println("出错了，....")
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "获取视频失败"})
	}

	response := FavListResponse{
		StatusCode: 0, // 成功状态码
		StatusMsg:  "获取视频成功",
		VideoList:  resp,
	}

	ctx.JSON(http.StatusOK, response)
}

// 获取视频id
func GetVideoId(userID int64) []int64 {
	var videoId []int64
	//db, _ := sql.Open(dbName, dbConnect)
	/*sql := fmt.Sprintf("select video_id from favorite where user_id=%d", userID)
	rows, err := database.DB.Query(sql)
	if err != nil {
		log.Println(err)
		return videoId, err
	}
	defer rows.Close()
	for rows.Next() {
		var vid int64
		// 获取各列的值，放到对应的地址中
		rows.Scan(&vid)
		videoId = append(videoId, vid)
	}*/
	//defer db.Close()
	fmt.Println(userID)
	err := database.DB.Table("favorite").Where("user_id=?", userID).Pluck("video_id", &videoId).Error
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(videoId)
	return videoId
}

// 获取视频实例
func GetVideoById(videoId []int64) ([]models.Video, error) {
	var videos []models.Video
	for _, id := range videoId {
		fmt.Println(id)
		var v models.Video
		database.DB.Table("video").Where("video_id=?", id).Find(&v)
		fmt.Println(v)
		videos = append(videos, v)
	}
	return videos, nil
}
