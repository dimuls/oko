# oko

Система по идентификации пользователя по фото/видео изображению. Реализован в рамках хакатона 
[Fintech&Security Superhero](https://dshkazan.ru/finsec) командой "Чёрный лебедь".

[Видео с защитой проекта](https://youtu.be/SYFwSqqjjSU).

[Презентация системы](https://github.com/dimuls/oko/blob/master/presentation.pdf).

Краткое описание Go-пакетов и папок этого репозитория приведены ниже.

## [agent](https://github.com/dimuls/oko/tree/master/agent)

Содержит скрипт Агента на питоне.

## [entity](https://github.com/dimuls/oko/tree/master/entity)

Go-пакет, содержащий общие сущности.

## [face](https://github.com/dimuls/oko/tree/master/face)

Go-пакет, содержащий реализацию клиента face API. По просьбе эксперта основной код удалён.

## [overseer](https://github.com/dimuls/oko/tree/master/overseer)

Go-пакет, содержащий реализацию Надзирателя в виде Windows службы.

## [users](https://github.com/dimuls/oko/tree/master/users)

Go-пакет, содержащий реализацию утилиты users для управления пользователями в Надзирателе.