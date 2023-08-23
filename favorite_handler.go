package v1

import (
	//"database/sql"
	"douyin/database"
	"douyin/database/models"
	"fmt"
	"sync"
	"time"

	//"log"
	"net/http"
	"strconv"

	"douyin/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	//"sync"
)

//const dbName string = "mysql"
//const dbConnect string = "root:123456@tcp(127.0.0.1:3306)/douyin?charset=utf8&parseTime=true" //设置数据库连接参数

func Response(ctx *gin.Context, httpStatus int, v interface{}) {
	ctx.JSON(httpStatus, v)
}

func ErrorResponse(ctx *gin.Context, statusCode int, errorMsg string) {
	Response(ctx, statusCode, gin.H{
		"status_code": statusCode,
		"status_msg":  errorMsg,
	})
}

func SuccessResponse(ctx *gin.Context) {
	Response(ctx, http.StatusOK, gin.H{
		"status_code": 0,
		"status_msg":  "success",
	})
}

// FavActionParams是获取点赞操作请求的结构体
type FavActionParams struct {
	Token      string `form:"token" binding:"required"`
	VideoId    int64  `form:"video_id" binding:"required"`
	ActionType int8   `form:"action_type" binding:"required,oneof=1 2"`
}

// FavListResponse是获取点赞列表响应的结构体
type FavListResponse struct {
	StatusCode int              `json:"status_code"`
	StatusMsg  string           `json:"status_msg"`
	VideoList  []models.VideoFA `json:"video_list"`
}

// 点赞视频
func FavoriteAction(ctx *gin.Context) {
	var favInfo FavActionParams
	err := ctx.ShouldBind(&favInfo)
	if err != nil {
		ErrorResponse(ctx, http.StatusBadRequest, err.Error())
		return
	}
	tokenUids, _ := ctx.Get("user_id")
	tokenUid, _ := tokenUids.(int64)

	if err != nil {
		ErrorResponse(ctx, http.StatusBadRequest, err.Error())
		return
	}

	redisPool := middleware.RedisPool
	if redisPool != nil {
		fmt.Println("get")
	}
	err = FavoriteActionDo(tokenUid, favInfo.VideoId, favInfo.ActionType, redisPool)

	if err != nil {
		ErrorResponse(ctx, http.StatusBadRequest, err.Error())
		return
	}
	SuccessResponse(ctx)
	// 获取 Redis 连接池

}

func FavoriteActionDo(uid, vid int64, action int8, redisPool *redis.Pool) error {
	conn := redisPool.Get() //重用已有的连接
	defer conn.Close()
	// 获取视频对应的用户ID
	authoruid, _ := redis.Int64(conn.Do("HGET", "video:"+strconv.FormatInt(vid, 10), "author_user_id"))

	db := database.DB.Begin() //开启数据库事务

	defer func() { //协程中发生 panic 时
		if r := recover(); r != nil {
			db.Rollback() //回滚
		}
	}()

	var wg sync.WaitGroup //等待所有协程完成
	wg.Add(4)             //启动4个协程

	act := action == 1 // 设置 act 为 true 或 false

	go func() {
		FavoriteTableChange(db, "favorite", uid, vid, act)
		defer wg.Done() //表示协程已经完成了其任务
	}()

	go func() {
		ChangeUserFavoriteCount(db, "user", uid, "favorite_count", act)
		defer wg.Done() //表示协程已经完成了其任务
	}()

	go func() {
		ChangeUserFavoriteCount(db, "user", authoruid, "total_favorited", act)
		defer wg.Done() //表示协程已经完成了其任务
	}()

	go func() {
		ChangeVideoLikesCount(db, "video", vid, act)
		defer wg.Done() //表示协程已经完成了其任务
	}()

	wg.Wait() // 等待所有协程完成

	if err := db.Commit(); err != nil { //db.Commit()提交数据库的更改
		fmt.Println("提交数据库修改时错误", err)
	}

	CacheFavoriteAction(uid, vid, act, redisPool)

	fmt.Println("All database operations completed.")
	return nil

}

func CacheFavoriteAction(uid, vid int64, action bool, redisPool *redis.Pool) error {
	conn := redisPool.Get() //重用已有的连接
	defer conn.Close()

	var authoruid int64
	// 获取视频对应的用户ID
	authoruid, err := redis.Int64(conn.Do("HGET", "video:"+strconv.FormatInt(vid, 10), "author_user_id"))

	if err != nil {
		fmt.Println(err)
		return err
	}

	if redisPool != nil {
		fmt.Println("Get conn!")
	}
	//使用 Redis 的哈希（Hash）来存储用户表和视频表的信息，使用集合（Set）来存储点赞关系。

	if action {
		conn.Send("SADD", "user:"+strconv.FormatInt(uid, 10)+":likes", vid)                  //存储点赞关系
		conn.Send("HINCRBY", "user:"+strconv.FormatInt(uid, 10), "favorite_count", 1)        //增加用户点赞数
		conn.Send("HINCRBY", "video:"+strconv.FormatInt(vid, 10), "likes_count", 1)          //增加用户点赞量
		conn.Send("HINCRBY", "user:"+strconv.FormatInt(authoruid, 10), "total_favorited", 1) //增加视频作者的被点赞量
		fmt.Println("cache ok!")
	} else {
		conn.Send("SREM", "user:"+strconv.FormatInt(uid, 10)+":likes", vid)
		conn.Send("HINCRBY", "user:"+strconv.FormatInt(uid, 10), "favorite_count", -1)
		conn.Send("HINCRBY", "video:"+strconv.FormatInt(vid, 10), "likes_count", -1)
		conn.Send("HINCRBY", "user:"+strconv.FormatInt(authoruid, 10), "total_favorited", -1)
		fmt.Println("cache ok!")
	}
	conn.Flush()
	return nil
}

// 更新favorite表，1代表取消点赞，-1代表未取消点赞
func FavoriteTableChange(db *gorm.DB, tableName string, userID int64, videoID int64, action bool) {
	var fav models.Favorite
	err := db.Table(tableName).Where("user_id = ? AND video_id = ?", userID, videoID).First(&fav).Error

	now := time.Now()

	if action {
		if gorm.IsRecordNotFoundError(err) {
			// 插入新记录，is_deleted 为 0，created_at 和 update_at 为当前时间
			newFav := models.Favorite{
				UserID:    userID,
				VideoID:   videoID,
				IsDeleted: -1,
				CreatedAt: now,
				UpdatedAt: now,
			}
			err = db.Table(tableName).Create(&newFav).Error
			if err != nil {
				fmt.Println("插入记录失败:", err)
				panic(err) //return
			}
			//fmt.Println("插入成功")
		} else {
			// 已点赞禁止再点赞
			if fav.IsDeleted == -1 {
				fmt.Println("操作错误!")
				panic(err) //return
			}
			// 更新记录的 is_deleted 和 updated_at 字段
			err = db.Model(&fav).Updates(models.Favorite{IsDeleted: -1, UpdatedAt: now}).Error
			if err != nil {
				fmt.Println("更新记录失败:", err)
				panic(err)
				//return
			}
			//fmt.Println("点赞更改记录成功")
		}
	} else {
		if gorm.IsRecordNotFoundError(err) {
			if err != nil {
				fmt.Println("操作错误:", err)
				panic(err) //return
			}
		} else {
			// 已取消点赞禁止再次取消点赞
			if fav.IsDeleted == 1 {
				fmt.Println("操作错误！")
				panic(err)
				//return
			}
			// 更新记录的 is_deleted 和 updated_at 字段
			err = db.Model(&fav).Updates(models.Favorite{IsDeleted: 1, UpdatedAt: now}).Error
			if err != nil {
				fmt.Println("更新记录失败:", err)
				panic(err)
				//return
			}
			//fmt.Println("取消点赞更改记录成功")
		}
	}
}

// 修改用户的 favorite_count 及 total_favorited
func ChangeUserFavoriteCount(db *gorm.DB, tableName string, userID int64, userORauther string, action bool) {
	var changeValue int
	if action {
		changeValue = 1
	} else {
		changeValue = -1
	}

	//Sql := fmt.Sprintf("update %s set favorite_count = favorite_count + %d where id = %d", tableName, changeValue, userID)
	//_, err = db.Exec(Sql)
	//err := database.DB.Table(tableName).Where("id=?", userID).Update("favorite_count", gorm.Expr("favorite_count+?", changeValue)).Error
	err := db.Table(tableName).Where("id=?", userID).Update(userORauther, gorm.Expr(userORauther+" + ?", changeValue)).Error
	if err != nil {
		fmt.Println("更新 favorite_count 失败:", err)
		panic(err)
	}

	//defer db.Close()
}

// 修改视频的 likes_count
func ChangeVideoLikesCount(db *gorm.DB, tableName string, videoID int64, action bool) {
	var changeValue int
	if action {
		changeValue = 1
	} else {
		changeValue = -1
	}

	//Sql := fmt.Sprintf("update %s set likes_count = likes_count + %d where video_id = %d", tableName, changeValue, videoID)
	//_, err = db.Exec(Sql)
	err := db.Table(tableName).Where("id=?", videoID).Update("likes", gorm.Expr("likes+?", changeValue)).Error
	if err != nil {
		fmt.Println("更新 likes_count 失败:", err)
		panic(err)
	}

	//defer db.Close()
}

// 获取点赞列表
func FavoriteList(ctx *gin.Context) {
	uID := ctx.Query("user_id")
	//token := ctx.Query("token")
	userID, _ := strconv.ParseInt(uID, 10, 64)

	videoId := GetVideoId(userID)
	//fmt.Println("videoId==", videoId)
	resp, err := GetVideoById(videoId)
	if err != nil {
		fmt.Println("出错了，获取视频失败")
		//ctx.JSON(http.StatusInternalServerError, gin.H{"error": "获取视频失败"})
		ErrorResponse(ctx, http.StatusBadRequest, err.Error())
		return
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
	fmt.Println(userID)
	err := database.DB.Table("favorite").Where("user_id=? AND is_deleted=-1", userID).Pluck("video_id", &videoId).Error
	if err != nil {
		fmt.Println(err)
	}
	//fmt.Println(videoId)
	return videoId
}

// 获取视频实例
func GetVideoById(videoId []int64) ([]models.VideoFA, error) {
	var videos []models.VideoFA
	for _, id := range videoId {
		fmt.Println(id)
		var v models.Video
		var u models.User
		database.DB.Table("video").Where("id=?", id).Find(&v)
		//fmt.Println(v)
		database.DB.Table("user").Where("id=?", v.AuthorUserID).Find(&u)
		//fmt.Println(u)
		userfa := models.UserFA{
			Avatar:          u.Avatar,
			BackgroundImage: u.BackgroundImage,
			FavoriteCount:   u.FavoriteCount,
			FollowCount:     u.FollowCount,
			FollowerCount:   u.FollowerCount,
			ID:              u.ID,
			IsFollow:        false,
			Name:            u.Name,
			Signature:       u.Signature,
			TotalFavorited:  strconv.FormatInt(u.TotalFavorited, 10),
			WorkCount:       u.WorkCount,
		}
		videofa := models.VideoFA{
			Author:        userfa,
			CommentCount:  int64(v.Comments),
			CoverURL:      v.CoverURL,
			FavoriteCount: int64(v.Likes),
			ID:            v.VideoID,
			IsFavorite:    true,
			PlayURL:       v.PlayURL,
			Title:         v.Title,
		}
		videos = append(videos, videofa)
	}
	return videos, nil
}
